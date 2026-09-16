# 云梦镜像边缘服务网络（service-edge）

> **云梦镜像边缘服务网络** 是项目正式名称，`service-edge` 为项目代号（用于代码、镜像、systemd 单元与安装路径等技术标识）。该项目来源于gs-global-mesh,ggm为轻量级分布式命令行工具,该项目在其基础上增加了前端页面,功能优化和拓展性,并通过MIT开源.

基于 FRP 的内网穿透管控系统（类 Cloudflare Tunnel 简化版）。统一控制面管理多个公网出口节点（frps）与内网客户端（frpc），通过 Web 控制台完成隧道的创建、部署与监控。

当前实现、问题分析与部署验收见 [Agent 与控制面重构说明](./docs/agent-control-plane.md)。`design.md` 为早期设计记录。

## 架构

```
控制面 (Go + SQLite + 嵌入式 React 前端)
   ├── 用户 API   (JWT, CORS 白名单)
   ├── Agent API  (节点独立凭据 + long-polling 配置下发)
   └── PKI        (CA 签发 frps/frpc 短期证书, 90 天, ≤30 天自动续签)
        ↑ HTTPS + Token
   ┌────┴─────┐         ┌──────────┐
 FRPS Agent  ───────────  FRPC Agent
 + frps 进程   frp 隧道    + frpc 进程
```

- **后端**：Go 1.26.2+ / Gin / GORM / SQLite（纯 Go `glebarez/sqlite`，无 CGO）
- **前端**：React 18 + TypeScript + Vite + Ant Design + TanStack Query + Zustand
- **Agent**：与后端复用代码，Linux amd64/arm64
- **FRP**：v0.61+ TOML 语法（`bindPort` / `auth.token` / `transport.tls.*` / `[[proxies]]`）。
  默认版本可在 `config.yaml` 配置；实际部署版本与架构必须先在设置页上传。

## 代码结构

```
cmd/server        控制面入口（含 gen-ca 子命令）
cmd/agent         Agent 入口（frps/frpc 共用）
internal/config   配置加载
internal/model    GORM 模型
internal/store    数据库访问、审计
internal/pki      CA 校验与证书签发
internal/service  业务逻辑（frps/frpc/proxy/enrollment/config 渲染/agent 协议）
internal/api      Gin 路由、handler、中间件（JWT/CORS/Agent Token）
internal/frp      部署路径约定、二进制安装、systemd、进程控制
internal/agent    Agent 主循环、long-polling、配置应用与回滚、watchdog
internal/protocol Agent↔后端 JSON 协议类型
internal/web      嵌入式前端 (go:embed dist)
scripts           安装脚本模板（嵌入渲染）
web               前端工程
```

## 快速开始（本地开发）

```bash
# 1. 生成开发用 CA
make dev-certs                 # 等价 go run ./cmd/server gen-ca --out dev

# 2. 准备配置（参考 config.example.yaml）
cp config.example.yaml config.yaml   # 修改 pki/数据库路径与各项 token

# 3. 构建并运行后端（嵌入前端需先 make web）
make web                       # 构建前端到 internal/web/dist
make server agents
./bin/service-edge --config config.yaml --agent-dist ./bin

# 4. 前端独立开发（带 /api 代理）
cd web && npm install && npm run dev    # http://127.0.0.1:5173
```

默认管理员由 `config.yaml` 的 `bootstrap_admin` 在首次启动时创建。

## 启动时强校验

控制面启动会强制校验 CA（文件可读、cert/key 配对、在有效期内、具备 CA 能力），任何一项失败立即 panic 退出，不允许带病启动。

## Agent 部署

控制台为每个 frps/frpc 生成一次性安装命令（15 分钟有效、单次使用）：

```bash
curl -fsSL "https://edge-api.dreamreflex.com/install/frps.sh?token=XXX" | sudo bash
```

脚本安装 Agent、写入节点独立凭据并注册；Agent 拉取完整配置后下载已上传的 frp 发行包，通过代际切换与重启应用配置，失败恢复上一代。

创建 FRPS 必须填写内网客户端可达的 IP/域名；HTTP/HTTPS 映射还需启用节点的对应入口端口。Agent 在线只表示心跳正常，连接详情分别显示代理注册状态和内网目标 TCP 可达性。

**此版本需配套更新控制面和 Agent。** 旧的全局 Agent 凭据不再接受，请在原节点页面重新生成安装命令并重装本版 Agent。

## 测试

```bash
go test ./...            # 单元测试（PKI 链校验、配置渲染等）
go vet ./...
```

真实 frp 本机集成测试与多机验收边界见 [回归验证](./docs/agent-control-plane.md#回归验证)，覆盖 TCP/UDP/HTTP/HTTPS 转发、断网恢复、回滚、状态文件丢失和删除清理。

## 安全要点

- Agent 凭据绑定角色与 UUID；主密钥只保留在控制面。
- frp token 节点级独立（64 字符随机）；TLS 双向校验，frpc 通过 `serverName=frps-<uuid>` 固定校验对端。
- 一次性安装 token：`UPDATE ... WHERE used_at IS NULL` 原子消费，唯一约束防重复。
- 前后端分离，使用 `Authorization: Bearer <jwt>`，不使用 Cookie，CORS 严格白名单。
- 所有写操作落审计日志。

## 发布与供应链安全

- **自动发布**：推送 `v*` 形式的 Git tag 触发 [`.github/workflows/release.yml`](./.github/workflows/release.yml)，自动构建前端、交叉编译控制面与 Agent 二进制（linux amd64/arm64）、生成 `SHA256SUMS`，并创建 GitHub Release。
- **构建溯源**：发布产物附带 [SLSA build provenance](https://slsa.dev/) 证明（`actions/attest-build-provenance`），可用 `gh attestation verify` 校验来源。
- **依赖治理**：[Dependabot](./.github/dependabot.yml) 每周检查 Go / npm / GitHub Actions 依赖更新；PR 经 [Dependency Review](./.github/workflows/dependency-review.yml) 拦截高危漏洞与不兼容许可。
- **代码扫描**：[CodeQL](./.github/workflows/codeql.yml) 对 Go 与 TypeScript 做安全与质量扫描。

## 许可

本项目以 [MIT 许可](./LICENSE) 开源，版权归 **dreamreflex** 所有。
