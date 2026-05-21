import { Card, Space, Typography } from 'antd'
import { GithubOutlined, CopyrightOutlined, InfoCircleOutlined } from '@ant-design/icons'

const { Title, Paragraph, Text, Link } = Typography

import { version } from '../../package.json'
const GITHUB_REPO = 'https://github.com/phil616/service-edge'
const LICENSE = 'MIT'
const COPYRIGHT = 'dreamreflex'

export default function About() {
  return (
    <Space direction="vertical" size={16} style={{ width: '100%', maxWidth: 920 }}>
      <Card title={<Title level={4} style={{ margin: 0 }}>关于</Title>}>
        <Paragraph>
          <Text strong>云梦镜像边缘服务网络</Text>（<Text code>service-edge</Text>）
          是一个基于 <Text code>frp</Text> 的边缘节点连接管理控制台。
          通过统一的 Web 控制面管理多个公网出口节点（FRPS）与内网客户端（FRPC），
          实现内网穿透隧道的创建、部署与监控。
        </Paragraph>
      </Card>

      <Card title={<Title level={4} style={{ margin: 0 }}><InfoCircleOutlined /> 项目信息</Title>}>
        <table style={{ width: '100%', borderCollapse: 'collapse' }}>
          <tbody>
            <tr>
              <td style={{ padding: '8px 16px 8px 0', whiteSpace: 'nowrap', color: 'rgba(0,0,0,0.45)' }}>版本</td>
              <td style={{ padding: '8px 0' }}>
                <Text code>{version}</Text>
              </td>
            </tr>
            <tr>
              <td style={{ padding: '8px 16px 8px 0', whiteSpace: 'nowrap', color: 'rgba(0,0,0,0.45)' }}>许可</td>
              <td style={{ padding: '8px 0' }}>
                <Text>{LICENSE}</Text>
              </td>
            </tr>
            <tr>
              <td style={{ padding: '8px 16px 8px 0', whiteSpace: 'nowrap', color: 'rgba(0,0,0,0.45)' }}>版权</td>
              <td style={{ padding: '8px 0' }}>
                <CopyrightOutlined style={{ marginRight: 4 }} />
                {COPYRIGHT}
              </td>
            </tr>
            <tr>
              <td style={{ padding: '8px 16px 8px 0', whiteSpace: 'nowrap', color: 'rgba(0,0,0,0.45)' }}>源码</td>
              <td style={{ padding: '8px 0' }}>
                <Link href={GITHUB_REPO} target="_blank">
                  <GithubOutlined style={{ marginRight: 4 }} />
                  {GITHUB_REPO}
                </Link>
              </td>
            </tr>
          </tbody>
        </table>
      </Card>

      <Card title={<Title level={4} style={{ margin: 0 }}>技术栈</Title>}>
        <Paragraph>
          <ul>
            <li><Text strong>后端</Text>：Go 1.22+ / Gin / GORM / SQLite</li>
            <li><Text strong>前端</Text>：React 18 + TypeScript + Vite + Ant Design</li>
            <li><Text strong>Agent</Text>：Linux amd64 / arm64，systemd 托管</li>
            <li><Text strong>隧道</Text>：frp v0.61+，TLS 双向校验，短期证书自动续签</li>
          </ul>
        </Paragraph>
      </Card>
    </Space>
  )
}
