# Agent / FRP 可靠性重构

## 已确认的问题

1. **安装配置与 Agent 校验不兼容**：v1.5.2 拒绝 `config_poll_timeout: 30s`，安装模板却仍生成 30s，导致新 Agent 启动即退出。这是项目回归。现在旧配置自动归一化为 60s；安装器用下载到的 Agent 验证实际 YAML。
2. **代理名跨连接冲突**：不同连接都使用 `ssh` 等名称时，frps 看到重复名称。现在以连接 UUID 作为 frp `user` 前缀，显示及状态匹配时移除该前缀；同一连接内重复名称在保存前拒绝。
3. **首次登录失败导致退出**：显式设置 `loginFailExit = false`，临时不可达由 frpc 自行重连。移除 Agent 中与 systemd 竞争的 watchdog 重启循环。
4. **配置、证书、二进制不属于同一次部署**：旧流程覆盖共享二进制，部分失败时无法完整回滚。现在每个实例使用独立部署目录，保存对应二进制、配置和证书，`config/current` 原子切换。Agent 自己的 YAML 不在切换目录内。
5. **配置写入与版本递增分离**：代理、连接、服务端变更及证书更新的版本递增纳入同一数据库事务；提交成功才唤醒长轮询。主机配置在同一事务快照内组装。
6. **Agent 在线与 frp 可用混淆**：收到认证心跳即证明 Agent 在线；frp 进程停止、采样失败、代理注册失败分别记录。首次应用失败也创建诊断状态。进程/连接轻量状态每 10s 上报，主机信息按配置周期采集。
7. **恢复时间缺少实际验证**：默认参数下真实 KCP 测试出现服务端重启后 45s 内无法恢复。客户端和服务端显式配置 `transport.tcpMuxKeepaliveInterval = 10`，客户端 TCP keepalive 为 30s；QUIC 设置 keepalive 5s、idle timeout 15s，避免重启后旧会话长时间占住代理名。恢复仍受协议超时和网络状况影响。
8. **WSS 被错误标记为直接可用**：真实测试无法连接，官方 frps 服务端也未提供原生 WSS 解封装入口；当前项目没有独立 TLS WebSocket 网关配置。因此拒绝新增 WSS，已有 WSS 连接明确报告配置错误，需改为 TCP 或 WebSocket（两者均启用双向 TLS）。参见 [官方 frps 服务端实现](https://github.com/fatedier/frp/blob/v0.68.0/server/service.go)。

frp 字段含义依据 [v0.61.1 官方客户端配置源代码](https://github.com/fatedier/frp/blob/v0.61.1/pkg/config/v1/client.go)。默认 `user` 为空、`loginFailExit` 为 true，TCPMux 保活间隔为 30s。

## 运行机制

- 心跳、配置同步和状态采集独立执行。采样与上报分别限时，采样失败仍能提交诊断。
- 下载使用可取消请求和版本缓存，不覆盖正在使用的可执行文件。候选文件先检查版本，再验证配置和证书。
- systemd 负责进程退出后的重启；Agent 负责配置变更和定期完整核对。相同配置不会重启健康进程。
- 部署进程稳定后记录 `activated`；若 Agent 在切换期间退出，下次应用先恢复上一次部署。保留当前和上一次已成功部署。
- “已应用”指本地部署成功，不表示远端隧道已经可用；连接是否可用由 frpc 管理 API 的实际代理状态判断。
- 删除连接先执行，不因其他连接的二进制下载失败而延迟。
- 缺少目标 frps 地址时，下发连接级配置错误；其余连接可以独立应用。

## 域名和资源地址

| 用途 | 配置 |
| --- | --- |
| Agent 控制平面地址 | `server.external_url`，必须是 Agent 可访问的 HTTP(S) 根地址，不附加 `/api/v1` |
| 安装脚本 | `install_script_base`；省略时为 `external_url + /install` |
| Agent 二进制 | `agent_download_base`；省略时为 `external_url + /download/agent`；系统设置可按角色覆盖 |
| frp 二进制 | 仅使用管理员上传的 `/api/v1/frp-dist/` 资源；缺少资源时拒绝下发配置，不会访问 GitHub；该资源必须返回原始 `.tar.gz`，不能被 SPA fallback 替换成 HTML |
| frpc 连接目标 | 节点 `public_ip`，可填写 IP 或域名；这是数据面地址，与 API 地址独立 |
| 浏览器 API | 前端构建的 API 配置；跨域部署时同步配置后端 CORS |

这些地址可配置，没有必须使用的业务域名。更换控制平面地址不会自动修改已安装 Agent 的 `agent.yaml`，需要更新并重启 Agent。反向代理必须允许超过 30s 的长轮询，建议读取超时至少 65s。Agent 需要能访问控制平面及 `/api/v1/frp-dist/`；管理员须先上传对应版本、系统和架构的归档。frpc 还需要访问 frps 对应 TCP/UDP 端口。

## 升级顺序

1. 备份控制平面数据库及 CA，发布新控制平面和新 Agent 下载产物。
2. 先升级一个 frps Agent，再升级少量 frpc Agent，验证配置版本、进程状态及实际转发。升级时会有一次受控进程重启。
3. 使用保持原 UUID 的安装命令升级其余 Agent；安装器拒绝直接替换不同 UUID，避免遗留连接。
4. 旧版 Agent 不会因为控制平面升级而自动更新自身。只升级控制平面无法获得本次部署和重连修复。

首次从旧部署迁移时保留可完整读取的旧配置与二进制；旧部署缺失文件时允许修复，但没有完整旧版本可回滚。安装器失败时恢复旧 Agent 二进制、配置、服务文件以及先前运行状态。

## 验证方式与边界

常规检查：

```sh
go test -race ./...
go vet ./...
(cd web && npm ci && npm run build)
```

真实 frp 测试使用官方发布的 Linux amd64 二进制，覆盖 v0.61.1 / v0.68.0，以及 TCP、KCP、QUIC、WebSocket。WSS 的拒绝逻辑由单元测试覆盖。解压后指定目录：

```sh
FRP_TEST_BIN_DIR=/path/to/frp_0.61.1_linux_amd64 \
  go test -race -tags integration ./internal/agent \
  -run TestRealFRP -v -count=1 -timeout=8m
```

测试运行真实 frps/frpc、项目生成的配置、双向 TLS、HTTP 转发和管理 API，检查：同名代理、先启动客户端、TCP 双向数据静默丢弃 35s 后恢复、服务器中断恢复、重复配置不重启、错误证书/配置拒绝、失败升级回滚、未完成激活恢复。`.github/workflows/reliability.yml` 提供可重复执行的版本矩阵。

测试中的 systemd 被子进程管理器替代，未验证生产主机的 systemd、实际 DNS、防火墙、运营商丢包或跨地域延迟，也不声称证明断电时文件系统持久性。本次结果应配合少量真实主机灰度及日志观察验收。

### 本次本地执行结果

- v0.61.1 / v0.68.0 的四种受支持传输均通过真实转发、管理 API、服务端中断恢复、失败回滚及中断激活恢复测试，并开启 Go race detector。
- 两个版本额外通过 TCP 双向静默丢包 35s 测试；恢复过程中 frpc PID 不变。
- 配置版本写入失败注入验证整个事务回滚；安装脚本在隔离路径和模拟 systemd 下验证失败恢复；下载验证错误版本拒绝、旧二进制保留、离线缓存及取消。
- `go test -race ./...`、`go vet ./...`、`go build ./...` 和前端构建通过。前端仍有既存的打包体积提示。

本次重构随 v1.5.3 发布；上线后仍需观察真实主机的网络和 systemd 行为。
