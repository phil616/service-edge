# 云梦镜像边缘服务网络（service-edge）技术方案

## 一、项目概述

### 1.1 项目定位

云梦镜像边缘服务网络（service-edge）是一套基于 FRP 的内网穿透管控系统，类似于 Cloudflare Tunnel 的简化版本。系统通过统一的控制面管理多个公网出口节点（frps）和客户端（frpc），用户通过 Web 控制台即可完成隧道的创建、部署、监控全流程。

### 1.2 安全假设

本系统假设运行在**受控环境**中，即所有部署节点都是可信的。系统不防御以下威胁：
- Agent 二进制和配置被恶意复制到其他机器
- 持有合法凭据的内部人员的恶意行为

### 1.3 域名规划

| 用途 | 域名 |
|---|---|
| 前端 Web 控制台 | `edge.dreamreflex.com` |
| 后端 API | `edge-api.dreamreflex.com` |

所有 Agent 通过 `edge-api.dreamreflex.com` 与控制面通信。

---

## 二、技术栈

### 2.1 后端

- 语言：Go（建议 1.22+）
- HTTP 框架：Gin 或 Echo
- ORM：GORM
- 持久化：SQLite（单文件数据库）
- 配置文件：YAML
- 认证：JWT（用户）+ Token（Agent）
- 加密：标准库 `crypto/tls`、`crypto/x509`

### 2.2 前端

- 框架：React 18+ with TypeScript
- 构建工具：Vite
- 路由：React Router
- 状态管理：Zustand 或 TanStack Query
- UI 库：Ant Design 或 shadcn/ui
- HTTP 客户端：Axios

### 2.3 Agent

- 语言：Go（与后端复用代码）
- 跨平台编译：Linux amd64/arm64

### 2.4 FRP 版本

**重要**：在编码开始前，必须通过 WebSearch 或访问 [FRP 官方文档](https://github.com/fatedier/frp) 确认最新版本（建议 v0.58+）的配置文件语法。v0.52+ 起 FRP 已从 `.ini` 切换到 `.toml`，且配置项命名风格变化较大（如 `bind_port` → `bindPort`）。**严禁基于旧语法编码**。

---

## 三、系统架构

### 3.1 三层架构

```
┌─────────────────────────────────────────────────┐
│  控制面（Control Plane）                          │
│  - 前端：edge.dreamreflex.com                    │
│  - 后端：edge-api.dreamreflex.com                │
│  - 持久化：SQLite                                │
└─────────────────────────────────────────────────┘
                ↑                  ↑
                │  long-polling    │  long-polling
                │  HTTPS + Token   │  HTTPS + Token
                │                  │
    ┌───────────┴────────┐   ┌─────┴──────────────┐
    │  FRPS Agent        │   │  FRPC Agent        │
    │  + frps 进程       │   │  + frpc 进程组     │
    │  （公网节点）      │   │  （内网客户端）    │
    └────────────────────┘   └────────────────────┘
            ↑                          │
            │  TCP + Token             │
            └──── frp 隧道 ────────────┘
```

### 3.2 关键组件清单

| 组件 | 部署位置 | 职责 |
|---|---|---|
| Control Plane Backend | 控制面服务器 | API、业务逻辑、配置下发 |
| Control Plane Frontend | 控制面服务器（静态托管） | 管理界面 |
| FRPS Agent | 各公网节点 | 管理 frps 进程、上报状态、拉取配置 |
| FRPC Agent | 各内网客户端 | 管理 frpc 进程组、上报状态、拉取配置 |
| frps | 各公网节点 | FRP 服务端，由 FRPS Agent 启停 |
| frpc | 各内网客户端 | FRP 客户端，由 FRPC Agent 启停 |

---

## 四、控制面设计

### 4.1 启动配置

控制面通过 `service-edge --config config.yaml` 启动，配置文件结构：

```yaml
server:
  listen: "0.0.0.0:8443"
  external_url: "https://edge-api.dreamreflex.com"
  
database:
  path: "/var/lib/service-edge/data.db"

# Agent 与控制面通信的认证 token（所有 Agent 共用此 token 鉴权 API）
# 注意：这是 Agent ↔ 控制面 API 的认证，不是 frpc ↔ frps 的认证
agent_api_token: "随机字符串，建议 64 字符"

# 用户登录的 JWT 签名密钥
jwt_secret: "随机字符串，建议 64 字符"

# 控制面持有的根证书或边缘证书
# 启动时必须能成功加载并验证，否则直接报错退出
pki:
  ca_cert: "/etc/service-edge/ca.crt"
  ca_key:  "/etc/service-edge/ca.key"

# frp 二进制下载地址（模板，{version} 会被替换）
frp_release:
  base_url: "https://github.com/fatedier/frp/releases/download"
  default_version: "v0.61.1"

# 安装命令的 base URL（生成的脚本会从这里下载）
install_script_base: "https://edge-api.dreamreflex.com/install"

# 一次性安装 token 的有效期
enrollment_token_ttl: "15m"

cors:
  allowed_origins:
    - "https://edge.dreamreflex.com"

logging:
  level: "info"
  path: "/var/log/service-edge/server.log"
```

### 4.2 启动时的强校验

1. 加载 `pki.ca_cert` 和 `pki.ca_key`，校验：
   - 文件存在且可读
   - 证书与私钥配对（公钥匹配）
   - 证书在有效期内
   - 证书具有 CA 能力（`BasicConstraints.IsCA = true`），或者是合法的中间证书
2. 校验数据库连接，自动执行迁移
3. 任何一项失败则**直接 panic 退出**，不允许带病启动

### 4.3 数据库表结构

```sql
-- 用户表（控制面登录用户）
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- FRPS 节点表
CREATE TABLE frps_nodes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,            -- frps 唯一标识
    name TEXT NOT NULL,                   -- 边缘节点名称
    bind_port INTEGER NOT NULL,           -- frpc 连接的端口
    dashboard_port INTEGER,               -- Web 管理端口，NULL 表示不启用
    dashboard_user TEXT,
    dashboard_pwd TEXT,
    frp_token TEXT NOT NULL,              -- 本节点专属的 frp token
    tls_cert TEXT NOT NULL,               -- frps 服务端证书（PEM）
    tls_key TEXT NOT NULL,                -- frps 服务端私钥（PEM）
    frp_version TEXT NOT NULL,
    config_version INTEGER DEFAULT 1,     -- 配置版本号
    status TEXT DEFAULT 'pending',        -- pending / online / offline
    last_heartbeat DATETIME,
    public_ip TEXT,                       -- Agent 上报或手动填写
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- FRPC 客户端表
CREATE TABLE frpc_clients (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT UNIQUE NOT NULL,            -- frpc 实例唯一标识（不是机器唯一）
    name TEXT NOT NULL,
    frps_uuid TEXT NOT NULL,              -- 连接的 frps 节点
    tls_cert TEXT NOT NULL,               -- frpc 客户端证书（PEM）
    tls_key TEXT NOT NULL,
    frp_version TEXT NOT NULL,
    config_version INTEGER DEFAULT 1,
    status TEXT DEFAULT 'pending',
    last_heartbeat DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (frps_uuid) REFERENCES frps_nodes(uuid)
);

-- 端口映射表（一个 frpc 可以有多条映射）
CREATE TABLE proxy_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    frpc_uuid TEXT NOT NULL,
    name TEXT NOT NULL,                   -- proxy 名称，frp 配置里用
    proxy_type TEXT NOT NULL,             -- tcp / udp / http / https
    local_ip TEXT DEFAULT '127.0.0.1',
    local_port INTEGER NOT NULL,
    remote_port INTEGER,                  -- TCP/UDP 用
    custom_domains TEXT,                  -- HTTP/HTTPS 用，JSON 数组
    subdomain TEXT,                       -- HTTP/HTTPS 用
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (frpc_uuid) REFERENCES frpc_clients(uuid)
);

-- 一次性安装 token 表
CREATE TABLE enrollment_tokens (
    token TEXT PRIMARY KEY,
    target_type TEXT NOT NULL,            -- 'frps' 或 'frpc'
    target_uuid TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at DATETIME,                     -- NULL 表示未使用
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 审计日志
CREATE TABLE audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    action TEXT NOT NULL,                 -- 'create_frps' / 'update_proxy' / ...
    target_type TEXT,
    target_uuid TEXT,
    detail TEXT,                          -- JSON
    ip TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 索引
CREATE INDEX idx_frps_uuid ON frps_nodes(uuid);
CREATE INDEX idx_frpc_uuid ON frpc_clients(uuid);
CREATE INDEX idx_frpc_frps ON frpc_clients(frps_uuid);
CREATE INDEX idx_proxy_frpc ON proxy_mappings(frpc_uuid);
CREATE INDEX idx_enrollment_expires ON enrollment_tokens(expires_at);
```

### 4.4 配置变更的事务性原则

涉及多步的操作（创建 frps、创建 frpc 等）必须在一个 SQL 事务里完成。文件类输出（证书、配置文件）不存磁盘，全部以 BLOB/TEXT 形式存数据库，由 Agent 拉取时实时生成磁盘文件。这样回滚只需要 ROLLBACK 数据库事务，无需清理磁盘残留。

### 4.5 证书签发逻辑

控制面持有 CA 后，需要实现以下证书签发能力：

| 用途 | Common Name 规则 | 有效期 |
|---|---|---|
| frps 服务端证书 | `frps-<uuid>` | 90 天 |
| frpc 客户端证书 | `frpc-<uuid>` | 90 天 |

**注意**：本期 frp token 是节点级共享密钥（一个 frps 对应一个 token，连接它的所有 frpc 用同一个 token），这是 FRP 原生机制，无法做到 frpc 级别的差异化认证。TLS 证书用于通信加密。

### 4.6 证书续签机制

- Agent 每次拉取配置时，控制面检查当前证书剩余有效期
- 剩余有效期 ≤ 30 天时，自动生成新证书，写回数据库，通过 long-polling 响应一并下发
- Agent 收到新证书后，原子替换本地文件并重启 frp 进程

---

## 五、API 设计

### 5.1 认证机制

**用户 API**（前端调用）：使用 JWT，前端通过 `Authorization: Bearer <jwt>` 请求头携带。CORS 白名单限制为 `edge.dreamreflex.com`。

**Agent API**：使用 `X-Agent-Token` 请求头携带控制面配置的 `agent_api_token`，外加请求路径里带 UUID 标识身份。

**一次性安装 API**：通过 URL query 参数携带 enrollment token，调用一次后立即失效。

### 5.2 用户 API（前端 → 后端）

```
# 认证
POST   /api/v1/auth/login                  用户登录
POST   /api/v1/auth/logout                 退出
GET    /api/v1/auth/me                     当前用户信息

# FRPS 管理
GET    /api/v1/frps                        列出所有 frps 节点
POST   /api/v1/frps                        创建 frps 节点
GET    /api/v1/frps/:uuid                  查询单个 frps
PUT    /api/v1/frps/:uuid                  更新 frps 配置
DELETE /api/v1/frps/:uuid                  删除 frps
POST   /api/v1/frps/:uuid/install-command  生成一次性安装命令
GET    /api/v1/frps/:uuid/status           查询运行状态

# FRPC 管理
GET    /api/v1/frpc                        列出所有 frpc
POST   /api/v1/frpc                        创建 frpc 实例
GET    /api/v1/frpc/:uuid                  查询单个 frpc
PUT    /api/v1/frpc/:uuid                  更新 frpc 配置
DELETE /api/v1/frpc/:uuid                  删除 frpc
POST   /api/v1/frpc/:uuid/install-command  生成一次性安装命令
GET    /api/v1/frpc/:uuid/status           查询运行状态

# 端口映射
GET    /api/v1/frpc/:uuid/proxies          列出该 frpc 的所有映射
POST   /api/v1/frpc/:uuid/proxies          新增映射
PUT    /api/v1/proxies/:id                 更新映射
DELETE /api/v1/proxies/:id                 删除映射

# 辅助
GET    /api/v1/frps/:uuid/available-ports  查询该 frps 的可用端口（排除已占用）
GET    /api/v1/audit-logs                  审计日志查询
```

### 5.3 Agent API（Agent → 后端）

所有 Agent API 必须携带：
- `X-Agent-Token: <agent_api_token>` （控制面配置的全局 token）
- `X-Agent-UUID: <uuid>` （Agent 自己的 UUID）
- `X-Agent-Type: frps|frpc`

```
# 心跳（高频，10-30s 一次）
POST   /api/v1/agent/heartbeat
请求体：{ "config_version": 5, "process_alive": true }
响应：{ "ok": true }

# 状态上报（低频，1-5 分钟一次）
POST   /api/v1/agent/status
请求体：{
  "config_version": 5,
  "process_alive": true,
  "process_pid": 12345,
  "frp_version": "v0.61.1",
  "system_info": {
    "os": "linux",
    "arch": "amd64",
    "kernel": "5.15.0",
    "memory_mb": 2048,
    "uptime_sec": 86400
  },
  "frp_status": {
    "active_connections": 3,
    "last_error": ""
  },
  "config_summary": {
    "proxy_count": 5
  }
}

# 拉取配置（long-polling）
GET    /api/v1/agent/config?current_version=5
请求会被服务端 hang 住最多 30 秒：
- 30 秒内有更新：立即返回 200 + 新配置
- 30 秒超时：返回 304 Not Modified，Agent 重新发起
响应（有更新时）：
{
  "config_version": 6,
  "frp_binary": {
    "version": "v0.61.1",
    "download_url": "https://github.com/.../frp_0.61.1_linux_amd64.tar.gz",
    "sha256": "..."
  },
  "frp_config": "<完整的 frps.toml / frpc.toml 内容>",
  "tls_cert": "-----BEGIN CERTIFICATE-----\n...",
  "tls_key": "-----BEGIN PRIVATE KEY-----\n...",
  "ca_cert": "-----BEGIN CERTIFICATE-----\n..."
}

# 上报配置应用结果
POST   /api/v1/agent/config/ack
请求体：{
  "config_version": 6,
  "success": true,
  "error": ""
}
若 success=false，控制面会记录错误，Agent 继续运行旧配置。

# 注册（首次安装时调用，需要一次性 enrollment token）
POST   /api/v1/agent/enroll?token=<enrollment_token>
请求体：{
  "uuid": "<自动从安装脚本里获取>",
  "agent_type": "frps|frpc",
  "system_info": { ... }
}
响应：成功后该 enrollment token 立即作废
```

### 5.4 安装脚本下载

```
GET /install/frps.sh?token=<enrollment_token>
GET /install/frpc.sh?token=<enrollment_token>
```

后端动态渲染脚本，把 UUID、API 地址、token 嵌入脚本。token 在脚本下载时不消耗，由 Agent 首次 enroll 时消耗。

---

## 六、Agent 设计

### 6.1 目录结构

#### FRPS Agent

```
/opt/service-edge/frps-agent/
├── bin/
│   ├── agent              # Agent 二进制
│   └── frps               # frps 二进制
├── config/
│   ├── agent.yaml         # Agent 自身配置（UUID、API 地址、token）
│   ├── frps.toml          # 当前 frps 配置
│   ├── frps.toml.new      # 待应用的新配置（reload 前的临时文件）
│   ├── server.crt
│   ├── server.key
│   └── ca.crt
├── data/
│   └── state.json         # Agent 状态（当前 config_version 等）
└── logs/
    ├── agent.log
    └── frps.log
```

#### FRPC Agent

```
/opt/service-edge/frpc-agent/
├── bin/
│   ├── agent
│   └── frpc               # 共享二进制
├── instances/
│   ├── <frpc-uuid-1>/
│   │   ├── config/
│   │   │   ├── frpc.toml
│   │   │   ├── frpc.toml.new
│   │   │   ├── client.crt
│   │   │   ├── client.key
│   │   │   └── ca.crt
│   │   ├── data/state.json
│   │   └── logs/frpc.log
│   └── <frpc-uuid-2>/
│       └── ...
├── config/
│   └── agent.yaml         # 一台机器一个 Agent，管理多个 frpc 实例
└── logs/
    └── agent.log
```

### 6.2 Agent 自身配置

```yaml
# /opt/service-edge/frps-agent/config/agent.yaml
agent_type: "frps"
uuid: "xxx-uuid-xxx"
api_endpoint: "https://edge-api.dreamreflex.com"
api_token: "<agent_api_token>"
heartbeat_interval: "20s"
status_report_interval: "180s"
config_poll_timeout: "30s"
frp_binary_path: "/opt/service-edge/frps-agent/bin/frps"
frp_config_path: "/opt/service-edge/frps-agent/config/frps.toml"
systemd_unit: "service-edge-frps"
```

### 6.3 Agent 主循环

每个 Agent 启动后并发运行四个 goroutine：

**心跳 goroutine**：每 `heartbeat_interval` 调用 `/agent/heartbeat`，超时不重试只记录。

**状态上报 goroutine**：每 `status_report_interval` 采集系统信息、frp 进程状态，调用 `/agent/status`。

**配置同步 goroutine**：使用 long-polling 调用 `/agent/config`：
1. 携带当前 `config_version`
2. 服务端 hang 最多 30s
3. 收到新配置 → 进入"配置应用"流程
4. 收到 304 → 立即重新发起请求
5. 网络错误 → 指数退避后重试（最大 60s）

**Watchdog goroutine**：每 30s 检查 frp 进程是否存活，若意外退出且未到达重启阈值（如 5 分钟内不超过 3 次），通过 systemd 重启。

### 6.4 配置应用流程（关键流程）

收到新配置后必须按以下顺序执行，任何一步失败都回滚：

```
1. 写新配置到 frp.toml.new（不动 frp.toml）
2. 写新证书到 cert.new、key.new
3. 执行 frps verify -c frp.toml.new 校验配置语法（FRP 提供）
   失败 → 删除 .new 文件，上报 ack(success=false)，继续运行旧配置
4. 备份当前 frp.toml → frp.toml.backup
5. 原子 rename frp.toml.new → frp.toml
   原子 rename cert.new → cert
   原子 rename key.new → key
6. 触发 frp 重新加载：
   - 优先尝试 SIGHUP（frp v0.50+ 支持热重载）
   - 失败则 systemctl restart
7. 等待 5 秒，检查进程是否存活、是否能响应
   失败 → 从 backup 恢复，重启进程，上报 ack(success=false)
8. 更新本地 state.json 的 config_version
9. 上报 ack(success=true)
```

### 6.5 配置变更的版本控制

- 控制面每次更新配置时，`config_version` 递增
- Agent 心跳和拉取配置时都携带当前 `config_version`
- 控制面对比，决定是否在 long-polling 响应中下发新版本
- Agent ack 时上报实际应用的版本

### 6.6 离线降级

Agent 失去控制面连接时的行为：
- frp 进程继续运行，不停止
- Agent 继续 watchdog 工作，保证 frp 进程存活
- 配置同步循环持续重试，指数退避
- 心跳和状态上报失败仅记录日志，不影响主流程

---

## 七、frp 配置模板

**重要前置**：以下配置模板基于 FRP v0.58+ 的 TOML 语法。编码前必须通过 WebSearch 确认目标版本的实际语法，避免使用已废弃的字段。

### 7.1 frps.toml 模板（控制面渲染）

```toml
bindPort = {{ .BindPort }}

auth.method = "token"
auth.token = "{{ .FrpToken }}"

transport.tls.force = true
transport.tls.certFile = "/opt/service-edge/frps-agent/config/server.crt"
transport.tls.keyFile = "/opt/service-edge/frps-agent/config/server.key"

{{ if .DashboardPort }}
webServer.addr = "0.0.0.0"
webServer.port = {{ .DashboardPort }}
webServer.user = "{{ .DashboardUser }}"
webServer.password = "{{ .DashboardPwd }}"
{{ end }}

log.to = "/opt/service-edge/frps-agent/logs/frps.log"
log.level = "info"
log.maxDays = 7
```

### 7.2 frpc.toml 模板（控制面渲染）

```toml
serverAddr = "{{ .ServerAddr }}"
serverPort = {{ .ServerPort }}

auth.method = "token"
auth.token = "{{ .FrpToken }}"

transport.tls.enable = true
transport.tls.certFile = "/opt/service-edge/frpc-agent/instances/{{ .Uuid }}/config/client.crt"
transport.tls.keyFile = "/opt/service-edge/frpc-agent/instances/{{ .Uuid }}/config/client.key"
transport.tls.trustedCaFile = "/opt/service-edge/frpc-agent/instances/{{ .Uuid }}/config/ca.crt"

log.to = "/opt/service-edge/frpc-agent/instances/{{ .Uuid }}/logs/frpc.log"
log.level = "info"
log.maxDays = 7

{{ range .Proxies }}
[[proxies]]
name = "{{ .Name }}"
type = "{{ .Type }}"
localIP = "{{ .LocalIP }}"
localPort = {{ .LocalPort }}
{{ if .RemotePort }}remotePort = {{ .RemotePort }}{{ end }}
{{ if .CustomDomains }}customDomains = {{ .CustomDomains | toJsonArray }}{{ end }}
{{ if .Subdomain }}subdomain = "{{ .Subdomain }}"{{ end }}
{{ end }}
```

---

## 八、安装脚本

### 8.1 FRPS 安装脚本（动态生成）

```bash
#!/bin/bash
set -euo pipefail

# 控制面渲染时注入以下变量
AGENT_UUID="{{ .Uuid }}"
API_ENDPOINT="https://edge-api.dreamreflex.com"
API_TOKEN="{{ .ApiToken }}"
ENROLLMENT_TOKEN="{{ .EnrollmentToken }}"
AGENT_DOWNLOAD_URL="{{ .AgentDownloadUrl }}"
FRP_VERSION="{{ .FrpVersion }}"

INSTALL_DIR="/opt/service-edge/frps-agent"

# 1. 检查 root 权限
if [[ $EUID -ne 0 ]]; then
   echo "请以 root 用户运行此脚本"
   exit 1
fi

# 2. 检测架构
ARCH=$(uname -m)
case $ARCH in
    x86_64) FRP_ARCH="amd64" ;;
    aarch64) FRP_ARCH="arm64" ;;
    *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

# 3. 创建目录
mkdir -p $INSTALL_DIR/{bin,config,data,logs}

# 4. 下载 frps 二进制
FRP_TARBALL="frp_${FRP_VERSION#v}_linux_${FRP_ARCH}.tar.gz"
FRP_URL="https://github.com/fatedier/frp/releases/download/${FRP_VERSION}/${FRP_TARBALL}"
echo "下载 frps: $FRP_URL"
curl -fSL "$FRP_URL" -o /tmp/frp.tar.gz
tar -xzf /tmp/frp.tar.gz -C /tmp/
cp /tmp/frp_*/frps $INSTALL_DIR/bin/frps
chmod +x $INSTALL_DIR/bin/frps
rm -rf /tmp/frp.tar.gz /tmp/frp_*

# 5. 下载 Agent 二进制
echo "下载 Agent: $AGENT_DOWNLOAD_URL"
curl -fSL "$AGENT_DOWNLOAD_URL" -o $INSTALL_DIR/bin/agent
chmod +x $INSTALL_DIR/bin/agent

# 6. 写入 Agent 配置
cat > $INSTALL_DIR/config/agent.yaml <<EOF
agent_type: "frps"
uuid: "$AGENT_UUID"
api_endpoint: "$API_ENDPOINT"
api_token: "$API_TOKEN"
heartbeat_interval: "20s"
status_report_interval: "180s"
config_poll_timeout: "30s"
frp_binary_path: "$INSTALL_DIR/bin/frps"
frp_config_path: "$INSTALL_DIR/config/frps.toml"
systemd_unit: "service-edge-frps"
EOF

# 7. 创建 systemd unit（Agent）
cat > /etc/systemd/system/service-edge-frps-agent.service <<EOF
[Unit]
Description=Service Edge FRPS Agent
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/agent --config $INSTALL_DIR/config/agent.yaml
Restart=on-failure
RestartSec=10s
StandardOutput=append:$INSTALL_DIR/logs/agent.log
StandardError=append:$INSTALL_DIR/logs/agent.log

[Install]
WantedBy=multi-user.target
EOF

# 8. 创建 systemd unit（frps，由 Agent 控制）
cat > /etc/systemd/system/service-edge-frps.service <<EOF
[Unit]
Description=Service Edge FRPS
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/frps -c $INSTALL_DIR/config/frps.toml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF

# 9. 执行注册（首次拉取配置和证书）
echo "向控制面注册..."
curl -fSL -X POST \
  -H "X-Agent-Token: $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"uuid\":\"$AGENT_UUID\",\"agent_type\":\"frps\"}" \
  "$API_ENDPOINT/api/v1/agent/enroll?token=$ENROLLMENT_TOKEN"

# 10. 启动 Agent（Agent 会自动拉取配置并启动 frps）
systemctl daemon-reload
systemctl enable service-edge-frps-agent
systemctl start service-edge-frps-agent

echo ""
echo "==========================================="
echo "FRPS Agent 已安装并启动"
echo "UUID: $AGENT_UUID"
echo "日志: $INSTALL_DIR/logs/agent.log"
echo "请确保以下端口在防火墙/安全组中放行："
echo "  - frps 服务端口（详见控制面）"
echo "==========================================="
```

### 8.2 FRPC 安装脚本（动态生成）

主要差异：
- 安装路径：`/opt/service-edge/frpc-agent/`
- 检查 frpc 二进制是否已存在，存在则跳过下载
- 多实例支持：以 UUID 为子目录组织配置
- systemd unit 使用模板：`service-edge-frpc@<uuid>.service`

```bash
# ... 前置部分相同 ...

# frpc 二进制路径固定，检测复用
FRPC_BINARY="/opt/service-edge/frpc-agent/bin/frpc"
if [ ! -f "$FRPC_BINARY" ]; then
    echo "下载 frpc..."
    # 下载逻辑同上
else
    # 检查版本，必要时升级
    EXISTING_VERSION=$("$FRPC_BINARY" --version 2>/dev/null || echo "unknown")
    echo "已存在 frpc: $EXISTING_VERSION"
fi

# 实例目录
INSTANCE_DIR="/opt/service-edge/frpc-agent/instances/$AGENT_UUID"
mkdir -p $INSTANCE_DIR/{config,data,logs}

# frpc 的 systemd template unit（只在 Agent 不存在时创建）
if [ ! -f /etc/systemd/system/service-edge-frpc@.service ]; then
cat > /etc/systemd/system/service-edge-frpc@.service <<EOF
[Unit]
Description=Service Edge FRPC Instance %i
After=network.target

[Service]
Type=simple
ExecStart=$FRPC_BINARY -c /opt/service-edge/frpc-agent/instances/%i/config/frpc.toml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5s

[Install]
WantedBy=multi-user.target
EOF
fi

# Agent 也是单例（一台机器只跑一个 Agent，管理多个 frpc 实例）
if [ ! -f /etc/systemd/system/service-edge-frpc-agent.service ]; then
    # 创建 Agent unit
fi
```

### 8.3 安装命令的最终形态

控制面生成的命令复制到剪贴板，用户在目标机器执行：

```bash
curl -fsSL "https://edge-api.dreamreflex.com/install/frps.sh?token=abc123xyz" | sudo bash
```

---

## 九、前端设计

### 9.1 页面结构

```
/login                         登录页
/                              Dashboard（节点总览）
/frps                          FRPS 节点列表
/frps/new                      创建 FRPS
/frps/:uuid                    FRPS 详情（状态、配置、安装命令）
/frpc                          FRPC 列表
/frpc/new                      创建 FRPC（选择目标 FRPS、配置映射）
/frpc/:uuid                    FRPC 详情（状态、映射列表、安装命令）
/audit-logs                    审计日志
/settings                      系统设置
```

### 9.2 创建 FRPS 表单字段

- 边缘节点名称（必填）
- 服务端口（必填，默认 7000，提示"需在防火墙开放此 TCP 端口"）
- Web 端口（选填，留空则不启用 Dashboard）
- Web 用户名（启用 Dashboard 时必填）
- Web 密码（启用 Dashboard 时必填）
- FRP 版本（下拉选择，默认控制面配置的 default_version）

提交成功后跳转到详情页，展示一次性安装命令、剩余有效期倒计时、复制按钮。

### 9.3 创建 FRPC 表单字段

- 客户端名称（必填）
- 目标 FRPS 节点（下拉选择，只显示 status=online 的节点）
- FRP 版本（下拉选择）
- 端口映射列表（可动态添加多条）：
  - 映射名称（必填，frps 上唯一）
  - 协议类型（TCP / UDP / HTTP / HTTPS）
  - 本地 IP（默认 127.0.0.1）
  - 本地端口（必填）
  - 远程端口（TCP/UDP 必填，前端实时校验该 FRPS 上是否已被占用）
  - 自定义域名（HTTP/HTTPS 用）
  - 子域名（HTTP/HTTPS 用）

提交成功后跳转到详情页，展示一次性安装命令和最终访问地址（如 `公网IP:远程端口`）。

### 9.4 节点状态展示

每个节点卡片显示：
- 名称、UUID（缩写）
- 状态徽章：online（绿）/ offline（红）/ pending（灰）
- 最后心跳时间
- 当前 config_version
- 操作按钮：详情、删除、重新生成安装命令

---

## 十、关键安全要点

### 10.1 控制面证书校验

启动时若 CA 证书加载失败、证书过期、私钥不匹配，**必须直接退出**，禁止以"降级模式"启动。

### 10.2 一次性安装 token

- 有效期 15 分钟
- 单次使用：消耗后立即写 `used_at`
- 数据库唯一约束防止重复消耗
- 服务端实现要原子化（事务 + UPDATE WHERE used_at IS NULL）

### 10.3 CORS 与 Cookie

- 前后端不同域名，使用 `Authorization: Bearer <jwt>` 而非 Cookie
- CORS `Access-Control-Allow-Origin` 严格限制为 `edge.dreamreflex.com`
- 不使用 `Access-Control-Allow-Credentials: true`，避免 Cookie 跨域风险

### 10.4 frp Token 隔离

每个 frps 节点的 token 独立生成（64 字符随机串），存数据库。一个节点 token 泄漏不影响其他节点。

### 10.5 审计日志

所有"写操作"必须落审计：
- 创建/删除/修改 FRPS、FRPC、Proxy
- 生成安装命令
- 用户登录
- 配置应用失败

---

## 十一、目录结构（项目代码）

```
service-edge/
├── cmd/
│   ├── server/main.go              # 控制面后端入口
│   └── agent/main.go               # Agent 入口（frps/frpc 共用）
├── internal/
│   ├── config/                     # 配置加载
│   ├── api/
│   │   ├── handler/                # HTTP handlers
│   │   ├── middleware/             # JWT、CORS、Agent Token 校验
│   │   └── router.go
│   ├── model/                      # GORM 模型
│   ├── service/                    # 业务逻辑
│   │   ├── frps.go
│   │   ├── frpc.go
│   │   ├── proxy.go
│   │   ├── enrollment.go
│   │   └── config_renderer.go      # frp.toml 模板渲染
│   ├── pki/                        # 证书签发
│   ├── agent/
│   │   ├── runner.go               # Agent 主循环
│   │   ├── heartbeat.go
│   │   ├── status.go
│   │   ├── config_sync.go          # long-polling
│   │   ├── applier.go              # 配置应用 + 回滚
│   │   └── watchdog.go
│   ├── frp/
│   │   ├── installer.go            # 二进制下载
│   │   ├── systemd.go              # systemd unit 管理
│   │   └── process.go              # 进程控制（SIGHUP/restart）
│   └── store/                      # 数据库访问层
├── web/                            # 前端项目
│   ├── src/
│   │   ├── pages/
│   │   ├── components/
│   │   ├── api/
│   │   └── store/
│   ├── package.json
│   └── vite.config.ts
├── scripts/
│   ├── install-frps.sh.tmpl        # 安装脚本模板
│   └── install-frpc.sh.tmpl
├── migrations/                     # SQL 迁移
├── config.example.yaml
├── Makefile
├── go.mod
└── README.md
```

---

## 十二、开发执行清单（给 Claude Code）

### 阶段零：调研（**必须先完成**）

1. WebSearch "frp latest version github release"
2. WebSearch "frp toml configuration v0.58 v0.61"
3. 访问 https://github.com/fatedier/frp 确认最新版本
4. 访问 https://github.com/fatedier/frp/blob/master/conf/frps_full_example.toml
5. 访问 https://github.com/fatedier/frp/blob/master/conf/frpc_full_example.toml
6. 确认以下要点并在代码注释中标注：
   - 是否仍使用 `auth.token`，还是已变更
   - TLS 相关字段名是否变化
   - 是否支持 SIGHUP 热重载，frp 命令行 reload 接口是否可用
   - `proxies` 数组语法是否仍是 TOML 数组写法

### 阶段一：控制面后端骨架

1. 项目初始化、go.mod、目录结构
2. 配置加载（config.yaml）
3. 启动时 CA 证书校验
4. SQLite 初始化 + 迁移
5. JWT 用户认证
6. CORS 中间件

### 阶段二：核心业务

1. PKI 模块（CA 签发 frps/frpc 证书）
2. FRPS CRUD API
3. FRPC + Proxy CRUD API
4. frp.toml 模板渲染器
5. Enrollment token 生成与消耗
6. 安装脚本模板渲染

### 阶段三：Agent 协议

1. Agent API（heartbeat、status、config、ack）
2. Long-polling 实现（带超时机制）
3. 配置版本对比与下发逻辑
4. 证书续签逻辑

### 阶段四：Agent 程序

1. Agent 配置加载、UUID 识别
2. 心跳与状态上报
3. Long-polling 配置同步
4. 配置应用流程（含回滚）
5. frp 二进制下载与验证
6. systemd unit 生成与控制
7. 多实例支持（frpc）
8. Watchdog

### 阶段五：前端

1. 项目初始化（Vite + React + TS）
2. 路由与布局
3. 登录页
4. FRPS 管理页
5. FRPC 管理页（含端口映射动态表单）
6. 安装命令展示组件（含倒计时、复制）
7. 节点状态展示

### 阶段六：集成测试

1. 启动控制面 → 创建 FRPS → 在测试机执行安装命令 → 验证 frps 启动
2. 创建 FRPC + 映射 → 在另一台测试机执行安装命令 → 验证隧道连通
3. 修改配置 → 验证 long-polling 下发与重启
4. 模拟配置错误 → 验证回滚
5. 模拟控制面下线 → 验证 Agent 离线降级
6. 证书续签验证（手动调整有效期到 30 天内）

---

## 十三、必须由 Claude Code 在编码前确认的事项

1. **FRP 配置语法**：使用 WebSearch 获取目标版本的 `frps_full_example.toml` 和 `frpc_full_example.toml`，作为模板的唯一参考来源
2. **FRP reload 机制**：确认目标版本支持 SIGHUP 热重载还是必须重启
3. **systemd template unit 语法**：确认 `service-edge-frpc@%i.service` 写法在目标 Linux 发行版上可用
4. **Go 模块版本**：所有依赖锁定到当前最新稳定版
5. **前端组件库选型**：在 Ant Design 与 shadcn/ui 之间确认一个，统一使用

---

以上方案是 MVP 范围。可观测性（日志/指标/告警）、HA、自动化运维等增强项均不在本期范围内。