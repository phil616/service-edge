import { Alert, Button, Card, Space, Tag, Typography, message } from 'antd'
import { CopyOutlined } from '@ant-design/icons'

const { Title, Paragraph, Text } = Typography

const FRPS_UNINSTALL = `# 彻底卸载 FRPS（服务端 / 公网节点）
sudo systemctl disable --now service-edge-frps service-edge-frps-agent 2>/dev/null || true
sudo rm -f /etc/systemd/system/service-edge-frps.service /etc/systemd/system/service-edge-frps-agent.service
sudo systemctl daemon-reload
sudo rm -rf /opt/service-edge/frps-agent
echo "service-edge FRPS 已彻底卸载"`

const FRPC_UNINSTALL = `# 彻底卸载 FRPC（客户端 / 边缘节点，含该主机上的所有实例）
sudo systemctl disable --now service-edge-frpc-agent 2>/dev/null || true
for u in $(systemctl list-units --all --plain --no-legend 'service-edge-frpc@*' 2>/dev/null | awk '{print $1}'); do sudo systemctl disable --now "$u" 2>/dev/null || true; done
sudo rm -f /etc/systemd/system/service-edge-frpc-agent.service /etc/systemd/system/service-edge-frpc@.service
sudo systemctl daemon-reload
sudo rm -rf /opt/service-edge/frpc-agent
echo "service-edge FRPC 已彻底卸载"`

function CommandBlock({ command }: { command: string }) {
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(command)
      message.success('卸载命令已复制，请在目标主机以 root 执行')
    } catch {
      message.warning('无法访问剪贴板，请手动选择复制')
    }
  }
  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      <pre
        className="mono"
        onClick={copy}
        title="点击复制"
        style={{
          background: '#1f1f1f',
          color: '#e6e6e6',
          padding: 16,
          borderRadius: 6,
          overflowX: 'auto',
          cursor: 'pointer',
          margin: 0,
          whiteSpace: 'pre',
        }}
      >
        {command}
      </pre>
      <Button icon={<CopyOutlined />} onClick={copy}>
        复制卸载命令
      </Button>
    </Space>
  )
}

export default function Help() {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%', maxWidth: 920 }}>
      <Card title={<Title level={4} style={{ margin: 0 }}>系统说明</Title>}>
        <Paragraph>
          <Text strong>云梦镜像边缘服务网络</Text>（<Text code>service-edge</Text>）是一个基于 <Text code>frp</Text> 的边缘节点连接管理控制台。
          它把内网穿透中分散在各台机器上的 <Text code>frps</Text>（服务端）与 <Text code>frpc</Text>（客户端）
          统一收拢到一个面板里管理，避免手动逐台编辑配置文件、重启进程。
        </Paragraph>
        <Paragraph>主要能力：</Paragraph>
        <ul>
          <li>一键生成安装命令，在目标主机上自动下载并以 systemd 托管 frp 与 Agent。</li>
          <li>集中维护 FRPS 节点、FRPC 客户端及其端口映射，支持新增、编辑、删除。</li>
          <li>Agent 定时上报心跳与运行状态，配置变更后自动下发并热重载。</li>
        </ul>
      </Card>

      <Card title={<Title level={4} style={{ margin: 0 }}>工作原理</Title>}>
        <Paragraph>
          <Tag color="blue">FRPS</Tag> 部署在拥有公网 IP 的服务器上，作为穿透入口；
          <Tag color="green">FRPC</Tag> 部署在内网/边缘设备上，主动连接 FRPS 并把本地端口暴露出去；
          每台主机上的 <Tag>Agent</Tag> 负责按控制台下发的配置启停 frp 进程并回报状态。
        </Paragraph>
        <Paragraph type="secondary">
          安装目录：FRPS 位于 <Text code>/opt/service-edge/frps-agent</Text>，
          FRPC 位于 <Text code>/opt/service-edge/frpc-agent</Text>。
        </Paragraph>
      </Card>

      <Card title={<Title level={4} style={{ margin: 0 }}>卸载 Agent 与 frp</Title>}>
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          message="卸载分两步"
          description={
            <>
              <div>1. 在本控制台删除对应的 FRPS 节点 / FRPC 客户端记录，清理控制面配置。</div>
              <div>
                2. 在目标主机以 <Text code>root</Text> 执行下方命令，停止并删除 systemd 服务、frp 与 Agent 二进制及全部数据目录。
              </div>
            </>
          }
        />

        <Title level={5}>卸载 FRPS（公网服务端）</Title>
        <CommandBlock command={FRPS_UNINSTALL} />

        <div style={{ height: 24 }} />

        <Title level={5}>卸载 FRPC（边缘客户端）</Title>
        <Paragraph type="secondary" style={{ marginTop: 0 }}>
          注意：FRPC 的 frp 二进制与 Agent 在同一主机上被所有实例共享，下述命令会移除该主机上的<Text strong>全部</Text> FRPC 实例。
        </Paragraph>
        <CommandBlock command={FRPC_UNINSTALL} />
      </Card>
    </Space>
  )
}
