# 云梦镜像边缘服务网络（service-edge）

> **云梦镜像边缘服务网络** 是项目正式名称，`service-edge` 为项目代号（用于代码、镜像、systemd 单元与安装路径等技术标识）。该项目来源于gs-global-mesh,ggm为轻量级分布式命令行工具,该项目在其基础上增加了前端页面,功能优化和拓展性,并通过MIT开源.

基于 FRP 的内网穿透管控系统（类 Cloudflare Tunnel 简化版）。统一控制面管理多个公网出口节点（frps）与内网客户端（frpc），通过 Web 控制台完成隧道的创建、部署与监控。

实现依据见 [`design.md`](./design.md)。

## 架构

```
控制面 (Go + SQLite + 嵌入式 React 前端)
   ├── 用户 API   (JWT, CORS 白名单)
   ├── Agent API  (X-Agent-Token + long-polling 配置下发)
   └── PKI        (CA 签发 frps/frpc 短期证书, 90 天, ≤30 天自动续签)
        ↑ HTTPS + Token
   ┌────┴─────┐         ┌──────────┐
 FRPS Agent  ───────────  FRPC Agent
 + frps 进程   frp 隧道    + frpc 进程
```

- **后端**：Go 1.22+ / Gin / GORM / SQLite（纯 Go `glebarez/sqlite`，无 CGO）
- **前端**：React 18 + TypeScript + Vite + Ant Design + TanStack Query + Zustand
- **Agent**：与后端复用代码，Linux amd64/arm64
- **FRP**：v0.61+ TOML 语法（`bindPort` / `auth.token` / `transport.tls.*` / `[[proxies]]`）。
  默认版本可在 `config.yaml` 配置，最新稳定版为 v0.68.x。

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
make server
./bin/service-edge --config config.yaml

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

脚本下载 frp 与 agent 二进制、写入 systemd unit、向控制面注册，随后 Agent 通过 long-polling 拉取配置与证书，原子应用并按需热重载/重启 frp。

## 测试

```bash
go test ./...            # 单元测试（PKI 链校验、配置渲染等）
go vet ./...
```

完整的多机集成测试（真实 frps/frpc 隧道连通、配置回滚、离线降级、证书续签）需要测试主机，见 `design.md` 第十二章阶段六。

## 安全要点

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
