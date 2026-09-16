# Service Edge 卸载指南

## 已安装的 systemd 服务

| 服务名 | 类型 | 安装目录 |
|---|---|---|
| `service-edge-frpc-agent` | FRPC 客户端 Agent | `/opt/service-edge/frpc-agent/` |
| `service-edge-frpc@` | FRPC 实例模板 | `/opt/service-edge/frpc-agent/instances/<uuid>/` |
| `service-edge-frps-agent` | FRPS 服务端 Agent | `/opt/service-edge/frps-agent/` |
| `service-edge-frps` | FRPS 服务 | `/opt/service-edge/frps-agent/` |

## 完整卸载

```bash
# 停止并禁用所有相关服务
systemctl stop service-edge-frpc-agent service-edge-frps-agent service-edge-frps 2>/dev/null
systemctl disable service-edge-frpc-agent service-edge-frps-agent service-edge-frps 2>/dev/null

# 停止所有 frpc 实例（如果有）
systemctl list-units 'service-edge-frpc@*' --no-legend | awk '{print $1}' | xargs -r systemctl stop
systemctl list-unit-files 'service-edge-frpc@*' --no-legend | awk '{print $1}' | xargs -r systemctl disable

# 删除 unit 文件
rm -f /etc/systemd/system/service-edge-frpc-agent.service
rm -f /etc/systemd/system/service-edge-frpc@.service
rm -f /etc/systemd/system/service-edge-frps-agent.service
rm -f /etc/systemd/system/service-edge-frps.service

# 重载 systemd
systemctl daemon-reload

# 删除安装目录
rm -rf /opt/service-edge
```

## 仅卸载 FRPC

```bash
systemctl stop service-edge-frpc-agent 2>/dev/null
systemctl disable service-edge-frpc-agent 2>/dev/null
systemctl list-units 'service-edge-frpc@*' --no-legend | awk '{print $1}' | xargs -r systemctl stop
systemctl list-unit-files 'service-edge-frpc@*' --no-legend | awk '{print $1}' | xargs -r systemctl disable
rm -f /etc/systemd/system/service-edge-frpc-agent.service /etc/systemd/system/service-edge-frpc@.service
systemctl daemon-reload
rm -rf /opt/service-edge/frpc-agent
```

## 仅卸载 FRPS

```bash
systemctl stop service-edge-frps-agent service-edge-frps 2>/dev/null
systemctl disable service-edge-frps-agent service-edge-frps 2>/dev/null
rm -f /etc/systemd/system/service-edge-frps-agent.service /etc/systemd/system/service-edge-frps.service
systemctl daemon-reload
rm -rf /opt/service-edge/frps-agent
```
