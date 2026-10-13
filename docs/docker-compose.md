# Docker Compose 部署

Denova 提供官方 Docker 镜像 `ghcr.io/planetsider/denova`，支持 `linux/amd64` 与 `linux/arm64`，由 GitHub Actions 在 `master` 分支更新和版本 tag 推送时自动构建并推送。

镜像为单容器形态：前端已内嵌进 Go 后端，无需单独的前端服务或数据库；插件与游戏后端所需的 Node.js 22 已包含在镜像内。

## 部署步骤

1. 准备部署目录并获取仓库文件：

   ```bash
   git clone https://github.com/PlanetSider/denova.git
   cd denova
   ```

2. 配置登录凭据（容器无法使用桌面端「设置 → 访问」的首次配置流程，必须通过环境变量提供）。编辑 `docker-compose.yml`，修改 `DENOVA_REMOTE_ACCESS_USERNAME` 与 `DENOVA_REMOTE_ACCESS_PASSWORD`：

   ```yaml
   environment:
     DENOVA_REMOTE_ACCESS_USERNAME: admin
     DENOVA_REMOTE_ACCESS_PASSWORD: change-me   # 首次启动前务必修改
   ```

   密码会在启动时做 bcrypt 哈希且只存在于内存中，不写入任何配置文件。该环境变量优先级高于界面中设置的密码；如希望完全由界面管理密码，删除 compose 中的 `DENOVA_REMOTE_ACCESS_PASSWORD` 行并改用预置数据目录的方式。若在本仓库目录内部署，注意不要把真实密码提交到远程仓库。

3. 启动：

   ```bash
   docker compose up -d
   ```

4. 浏览器打开 `http://<主机地址>:8080`，使用 compose 文件中配置的用户名和密码登录。

## 数据持久化

`docker-compose.yml` 使用绑定挂载（不使用 Docker named volume）：

| 宿主路径 | 容器路径 | 内容 |
| --- | --- | --- |
| `./data/.denova` | `/data/.denova` | 用户配置、受管项目、Agent 会话、版本与素材 |
| `./data/workspace` | `/data/workspace` | 默认打开的写作工作区（书籍内容） |

升级或重建容器不会丢失 `./data` 中的数据；请自行对 `./data` 做备份。如需打开已有项目目录，将目录挂载进容器（例如 `- ./books:/books`）后在界面中按容器内路径（`/books/...`）打开。

## 升级

```bash
docker compose pull
docker compose up -d
```

容器化部署通过替换镜像升级，不要在容器内使用应用内置的「检查更新 / 安装更新」：该机制按便携安装包设计，会替换镜像内程序并导致版本状态不一致。可在界面「设置 → 通用」中关闭自动检查更新。

## 使用限制与说明

- 首次登录必须使用 compose 文件中配置的凭据；登录后浏览器会话保持 30 天。
- 通过公网或域名访问时，请使用 Caddy、Nginx 等反向代理提供 HTTPS，避免明文传输登录凭据。反代需要支持 SSE 与 WebSocket（终端、Agent 流式输出）。
- 经 HTTPS 反向代理访问时，须在 compose 中设置 `DENOVA_TRUST_PROXY: "true"`（仅在反代与容器直连、且代理会发送 `X-Forwarded-Host/Proto` 时开启；Caddy、Nginx、NAS 自带反代默认满足）。否则后端会因同源校验拒绝请求，页面提示「此请求的来源不受信任」并返回 403。直连 HTTP 访问保持 `false`。开启后所有远程用户仍需登录，仅本机（loopback）管理操作不受影响。
- 资源中心导入/导出、插件平台管理等操作默认仅限运行 Denova 的本机触发；容器部署请设置 `DENOVA_REMOTE_MANAGE: "true"` 放开（登录鉴权仍保留）。依赖系统桌面的操作（目录选择对话框、文件 reveal）在无桌面的容器内仍不可用，新建项目请使用手动输入路径。
- Native 模式开箱即用；Codex / Claude Code 运行时需要在镜像内另装对应 CLI 并挂载其凭据目录（例如 `~/.codex`），镜像默认不包含。
- 自动化浏览器工具依赖容器内的 Chrome/Chromium/Edge，镜像默认未安装；未安装时该工具不启用，其余功能不受影响。
- 部分本机管理操作（目录选择、文件 reveal、部分资源导入导出管理）按设计仅限运行 Denova 的本机使用，远程浏览器中会提示不可用，这是预期行为。
