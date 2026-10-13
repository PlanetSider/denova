package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"
)

const (
	LocalHTTPHost = "127.0.0.1"
	LANHTTPHost   = "0.0.0.0"
)

var (
	ErrRemoteAccessUsernameRequired = errors.New("remote access username required")
	ErrRemoteAccessPasswordRequired = errors.New("remote access password required")
)

// RemoteAccessConfig is the runtime subset needed by the HTTP access gate.
type RemoteAccessConfig struct {
	DataDir        string // Runtime-only root for browser credentials.
	AllowLANAccess bool
	Username       string
	PasswordHash   string
	// TrustedProxy lets the access gate trust X-Forwarded-Host/Proto from the
	// direct peer, for deployments where a reverse proxy terminates HTTPS.
	TrustedProxy bool
	// RemoteManage lifts the loopback-only restriction on machine-side
	// operations (resource import/export, plugin management) for headless
	// server deployments; browser authentication still applies.
	RemoteManage bool
}

func HTTPListenHost(allowLANAccess bool) string {
	if allowLANAccess {
		return LANHTTPHost
	}
	return LocalHTTPHost
}

func HTTPURL(host string, port int) string {
	if port <= 0 {
		port = 8080
	}
	return "http://" + host + ":" + strconv.Itoa(port)
}

func LocalHTTPURL(port int) string {
	return HTTPURL("localhost", port)
}

func LANHTTPURL(port int) string {
	return HTTPURL(LANAddress(), port)
}

func LANAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return LANHTTPHost
	}
	var addrs []net.Addr
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if addresses, err := iface.Addrs(); err == nil {
			addrs = append(addrs, addresses...)
		}
	}
	return selectLANAddress(addrs)
}

// Prefer private IPv4 addresses for device-to-device access. Benchmark networks
// (198.18.0.0/15) are also used by local proxy tunnels and must not be advertised.
func selectLANAddress(addrs []net.Addr) string {
	fallback := LANHTTPHost
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP.To4()
		if ip == nil || !ip.IsGlobalUnicast() || (ip[0] == 198 && (ip[1] == 18 || ip[1] == 19)) {
			continue
		}
		if ip.IsPrivate() {
			return ip.String()
		}
		if fallback == LANHTTPHost {
			fallback = ip.String()
		}
	}
	return fallback
}

// PrepareUserSettingsForWrite normalizes remote-access credentials before
// replacing the user-level settings file. Blank password input preserves the
// existing password hash.
func PrepareUserSettingsForWrite(existing, incoming Settings) (Settings, error) {
	if incoming.GameCreationDefaults != nil {
		return Settings{}, fmt.Errorf("game creation defaults belong to a Project")
	}
	out := preserveTerminalCommandRegistryPresence(incoming)
	if err := validateSettingsCheckpointGuidance(out); err != nil {
		return Settings{}, err
	}
	out.AgentQuickPrompts = normalizeAgentQuickPrompts(out.AgentQuickPrompts)
	out.AgentApprovalRules = NormalizeAgentApprovalRules(out.AgentApprovalRules)
	if err := ValidateAgentApprovalRules(out.AgentApprovalRules); err != nil {
		return Settings{}, err
	}
	if err := validateTerminalCommands(out.TerminalCommands); err != nil {
		return Settings{}, err
	}
	if err := validateAgentQuickPrompts(out.AgentQuickPrompts); err != nil {
		return Settings{}, err
	}
	out.RemoteAccessUsername = strings.TrimSpace(out.RemoteAccessUsername)
	if out.RemoteAccessPassword != "" {
		hash, err := HashRemoteAccessPassword(out.RemoteAccessPassword)
		if err != nil {
			return Settings{}, err
		}
		out.RemoteAccessPasswordHash = hash
	} else if out.RemoteAccessPasswordHash == "" {
		out.RemoteAccessPasswordHash = existing.RemoteAccessPasswordHash
	}
	out.RemoteAccessPassword = ""
	out.RemoteAccessPasswordSet = out.RemoteAccessPasswordHash != ""

	if out.AllowLANAccess != nil && *out.AllowLANAccess {
		if out.RemoteAccessUsername == "" {
			return Settings{}, ErrRemoteAccessUsernameRequired
		}
		if out.RemoteAccessPasswordHash == "" {
			return Settings{}, ErrRemoteAccessPasswordRequired
		}
	}
	return out, nil
}

func HashRemoteAccessPassword(password string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", ErrRemoteAccessPasswordRequired
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("生成远程访问密码哈希失败: %w", err)
	}
	return string(hash), nil
}

// Environment-provided passwords must map to one stable hash: issued browser
// sessions snapshot the hash and bcrypt salts randomly, so re-hashing on every
// config load or restart would invalidate every login. The hash is cached per
// process and persisted (hash only, never the plaintext) beside the session
// store, then validated against the current password before reuse.
var envPasswordHash struct {
	mu       sync.Mutex
	password string
	hash     string
}

const envPasswordHashFile = "remote-access-env-hash"

func resolveEnvPasswordHash(password, dataDir string) (string, error) {
	password = strings.TrimSpace(password)
	if password == "" {
		return "", ErrRemoteAccessPasswordRequired
	}
	envPasswordHash.mu.Lock()
	defer envPasswordHash.mu.Unlock()
	if password == envPasswordHash.password && envPasswordHash.hash != "" {
		return envPasswordHash.hash, nil
	}
	hash := ""
	cachePath := ""
	if strings.TrimSpace(dataDir) != "" {
		cachePath = filepath.Join(dataDir, envPasswordHashFile)
		if data, err := os.ReadFile(cachePath); err == nil {
			stored := strings.TrimSpace(string(data))
			if CheckRemoteAccessPassword(stored, password) {
				hash = stored
			}
		}
	}
	if hash == "" {
		generated, err := HashRemoteAccessPassword(password)
		if err != nil {
			return "", err
		}
		hash = generated
		if cachePath != "" {
			if err := os.MkdirAll(dataDir, 0o700); err == nil {
				// Best effort: without the file the hash still stays stable
				// for the lifetime of the process.
				_ = os.WriteFile(cachePath, []byte(hash+"\n"), 0o600)
			}
		}
	}
	envPasswordHash.password = password
	envPasswordHash.hash = hash
	return hash, nil
}

func CheckRemoteAccessPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
