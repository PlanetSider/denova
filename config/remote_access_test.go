package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEnvPasswordHashStability(t *testing.T) {
	dataDir := t.TempDir()
	first, err := resolveEnvPasswordHash("secret-pass", dataDir)
	if err != nil {
		t.Fatal(err)
	}
	// Repeated config loads resolve the identical hash.
	second, err := resolveEnvPasswordHash("secret-pass", dataDir)
	if err != nil || second != first {
		t.Fatalf("hash not stable in process: %q %q", first, second)
	}
	// The cache stores a bcrypt hash, never the plaintext password.
	raw, err := os.ReadFile(filepath.Join(dataDir, envPasswordHashFile))
	if err != nil || !strings.HasPrefix(string(raw), "$2") || strings.Contains(string(raw), "secret-pass") {
		t.Fatalf("cache file content: %q err=%v", raw, err)
	}
	// A fresh process reuses the persisted hash instead of re-salting.
	envPasswordHash.password, envPasswordHash.hash = "", ""
	reused, err := resolveEnvPasswordHash("secret-pass", dataDir)
	if err != nil || reused != first {
		t.Fatalf("persisted hash not reused: %q %q", first, reused)
	}
	// Changing the password rotates the hash and keeps it valid.
	changed, err := resolveEnvPasswordHash("new-pass", dataDir)
	if err != nil || changed == first {
		t.Fatalf("password change did not rotate hash: %q", changed)
	}
	if !CheckRemoteAccessPassword(changed, "new-pass") {
		t.Fatal("rotated hash does not match the new password")
	}
	// A persisted hash that still matches is preferred verbatim.
	envPasswordHash.password, envPasswordHash.hash = "", ""
	if err := os.WriteFile(filepath.Join(dataDir, envPasswordHashFile), []byte(first+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	restored, err := resolveEnvPasswordHash("secret-pass", dataDir)
	if err != nil || restored != first {
		t.Fatalf("valid persisted hash not reused: %q %q", first, restored)
	}
}

func TestSelectLANAddress(t *testing.T) {
	for _, test := range []struct {
		name string
		ips  []string
		want string
	}{
		{"proxy before WLAN", []string{"198.18.0.1", "192.168.3.22"}, "192.168.3.22"},
		{"benchmark range", []string{"198.19.255.254", "10.0.0.12"}, "10.0.0.12"},
		{"private before public", []string{"8.8.4.4", "172.16.1.2"}, "172.16.1.2"},
		{"unusable addresses", []string{"127.0.0.1", "0.0.0.0", "169.254.18.199", "224.0.0.1", "255.255.255.255", "::1", "fe80::1", "192.168.3.22"}, "192.168.3.22"},
		{"public interface fallback", []string{"198.18.0.1", "8.8.4.4"}, "8.8.4.4"},
		{"no usable interface", []string{"198.18.0.1", "198.19.0.1", "169.254.18.199"}, LANHTTPHost},
		{"no interfaces", nil, LANHTTPHost},
	} {
		t.Run(test.name, func(t *testing.T) {
			var addrs []net.Addr
			for _, address := range test.ips {
				addrs = append(addrs, &net.IPNet{IP: net.ParseIP(address)})
			}
			if got := selectLANAddress(addrs); got != test.want {
				t.Fatalf("selected LAN address = %q, want %q", got, test.want)
			}
		})
	}
}
