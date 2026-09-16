# Agent 与控制面重构说明

## 本次定位到的问题

用户反馈是“Agent 在线，但隧道不通”。未连接实际故障主机，因此不能把其中某一项认定为该部署的唯一根因；以下是代码中可复现的问题及对应修复。

| 原设计问题 | 影响 | 新行为 |
| --- | --- | --- |
| 用心跳请求来源自动填充 FRPS 地址 | 经反向代理、NAT 时可能给 frpc 下发错误地址 | 创建节点必须明确填写 frpc 能访问的 IP/域名，禁止包含 URL 协议或端口 |
| 支持创建 HTTP/HTTPS 映射，却没有 frps 的 vhost 入口配置 | 配置能保存，实际代理无法注册 | 增加 HTTP/HTTPS 入口端口、子域名根域，校验和预留端口，拒绝未启用入口的映射 |
| 轻量状态上报为空时保留上一次在线状态 | 断线重连期间仍显示旧的成功状态 | 每次 admin API 快照替换旧观察值，缺失代理标记未知 |
| `running` 仅证明代理已注册，没有探测内网目标 | frpc 正常运行，内网服务停止时仍难以定位 | 独立采集本地 TCP 可达性及拨号错误，失败使连接降级；UDP 显示未验证 |
| 全部 Agent 持有同一个 API 凭据 | 修改 UUID/类型请求头可跨主机取配置和密钥 | HMAC 派生角色和 UUID 绑定的凭据，主密钥只在控制面；请求还需已注册身份 |
| 仅凭 state.json 枚举受管连接 | 激活后崩溃或状态文件丢失可能留下孤儿隧道 | 合并磁盘实例目录和持久状态，只按完整目标快照清理 |
| 删除节点仅删除数据库记录 | 离线或仍运行的远端 frp 不会停止 | 事务中写持久停机指令，重连后停止并禁用 FRP，成功 ACK 后记录完成 |
| 空主机配置仍依赖 frp 安装包 | 安装包丢失时无法下发清空连接配置 | 空连接快照不依赖安装包；停机指令也不依赖证书和安装包 |
| 缺字段的 200 响应可被当成空主机快照 | `{}`、`null` 等响应可能误删实例 | 必须有有效版本和显式 connections 数组；仅 `[]` 表示清空 |
| Agent 版本高于恢复后的数据库版本时等待“更高版本” | 数据库恢复后无法及时协调 | 版本不相等即下发当前完整目标状态 |

## 当前职责与协议

- 控制面：保存目标配置，事务更新配置版本，签发 frp 证书，派生节点凭据，生成安装命令和完整快照，保存观察状态及停机确认。
- Agent：主动连接控制面，分别执行心跳、配置协调和状态采集；只有验证过的完整快照才允许删除实例。
- systemd：持有每个 frps/frpc 的服务单元与失败重启策略。控制面失联不停止已运行的数据面。
- frp：执行实际转发和重连。现有不可变配置/证书/二进制代际切换与失败回滚继续保留。

注册凭据由 `HMAC-SHA256(master, domain + role + UUID)` 派生。安装脚本不再携带主密钥；注册请求同时验证节点凭据和短期一次性安装令牌，丢失响应可以按原身份重试。正常请求拒绝未注册身份、跨 UUID/角色凭据及旧的全局凭据。

删除时先在数据库事务中保存 `AgentRetirement`，再删除资源。Agent 仍可认证，但只能拿到停机指令。已删除身份无法再次注册。远端停止或禁用失败时保留清理信息并重试；成功记录在设置页的“节点删除与远端清理”。停机指令保留用于离线重连与重复投递；Agent 本身继续运行，供确认重试使用，不自动卸载软件。

## 如何解释状态

1. **Agent 在线**：控制面最近收到心跳，只代表管理连接正常。
2. **目标 / 已应用配置**：判断配置是否落到远端。失败时先读配置应用错误。
3. **代理已注册**：frpc 已向 frps 注册对应代理；没有代理快照时显示未知。
4. **内网目标 TCP 可达**：Agent 从自己的网络环境能连接目标端口；连接拒绝或超时会显示具体错误。
5. **实际业务可用**：还需从使用者所在网络访问公网入口。公网防火墙、DNS、HTTP Host、HTTPS SNI、应用认证与响应内容不由 TCP 探测保证。

内网 TCP 检查每次状态采集最多并发 4 个目标，每个拨号预算 700ms，受连接采集总预算限制。未采到结果是未知；UDP 不通过“拨号成功”推断业务可达。这些连接检查可能在 SSH/TLS 等服务日志中出现空连接记录。

## 部署这个版本

本次不兼容旧的 Agent API 凭据。控制面和 Agent 必须配套更新；不要让新控制面安装脚本下载旧版 Agent。

```bash
make all
./bin/service-edge --config config.yaml --agent-dist ./bin
```

`make all` 先构建前端，再编译嵌入新前端的控制面、本机 Agent、Linux amd64/arm64 Agent。`--agent-dist ./bin` 提供安装脚本所需的 `agent_linux_amd64` 和 `agent_linux_arm64`。

1. 按 `config.example.yaml` 设置 CA、数据库、外部访问 URL 和随机主密钥；生产外部 URL 由 HTTPS 反向代理提供。
2. 在设置页上传目标版本/架构的官方 FRP `.tar.gz`，不再宣称自动回退到 GitHub。
3. 创建 FRPS，填写内网 Agent 能访问的地址及控制端口；使用 HTTP/HTTPS 映射前配置对应入口端口。HTTPS 映射透传 TLS，内网目标必须提供 HTTPS。
4. 创建 FRPC 主机；分别在对应机器运行安装命令，然后创建连接及映射。放行控制端口、需要的 TCP/UDP 映射端口以及 HTTP/HTTPS 入口端口。
5. 若保留旧数据库，在原节点/主机页面重新生成安装命令并重装本版 Agent，保持原 UUID。无需删除原实例目录。数据库新增字段和停机记录表由 GORM 自动迁移。
6. 检查配置应用、代理注册、内网目标和实际请求四层结果；不能以 Agent 在线作为验收。

不要在未确认远端停止前手工删除停机记录。离线机器无法立即执行删除，设置页会明确显示等待确认。

## 回归验证

2026-09-16 本地验证：全量 Go 竞态测试、`go vet`、前端构建和 Linux amd64/arm64 Agent 构建通过；FRP 0.61.1、0.68.0 两版的完整真实流量/故障恢复回归均通过。

```bash
go test -race ./... -timeout=3m
go vet ./...
cd web && npm run build

# FRP_TEST_BIN_DIR 指向官方解压目录；同级放原始 release tar.gz。
FRP_TEST_BIN_DIR=/path/frp_0.68.0_linux_amd64 \
  go test -race -tags integration ./internal/agent -run TestRealFRP -v -count=1 -timeout=8m
```

`TestRealFRPControlPlaneLifecycle` 覆盖真实 HTTP 注册、配置轮询、控制面安装包下载、Agent 协调、TCP/UDP/HTTP/HTTPS 数据转发、状态回传、内网服务停止后的降级、丢失 state.json 后清理孤儿、主机和 FRPS 删除后的停止确认。

`TestRealFRPDeploymentAndRecovery` 覆盖 TCP/KCP/QUIC/WebSocket 控制传输、两个同名代理并存、35 秒网络黑洞、frps 重启、配置/证书错误、失败二进制回滚和中断激活恢复。CI 已按 FRP 0.61.1 和 0.68.0 两个版本执行所有 `TestRealFRP*`。

本次构建使用原有 npm 锁文件，依赖审计仍报告 11 项告警（6 high、4 moderate、1 low）；本次未进行独立的依赖升级。

本机集成测试替换的是 systemd 适配器，frps/frpc、证书、配置、HTTP 控制面和数据流量均真实运行。它不代表已验证用户公网安全组、多机路由或生产 systemd 环境。

FRP 配置语义参照官方 [服务端配置](https://gofrp.org/en/docs/reference/server-configures/) 与 [客户端配置](https://gofrp.org/en/docs/reference/client-configures/)。状态采集使用 [frpc admin API](https://github.com/fatedier/frp/blob/v0.61.1/client/admin_api.go) 的代理注册状态与 `local_addr`。
