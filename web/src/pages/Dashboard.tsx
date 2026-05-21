import { Card, Col, Row, Statistic, Table, Tag, Typography } from 'antd'
import { ApiOutlined, CloudServerOutlined, SafetyCertificateOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getCAInfo, getTopology } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import { archLabel, formatUptime } from '../lib/format'
import type { FRPCHost, FRPSNode } from '../api/types'

const { Title } = Typography

export default function Dashboard() {
  const navigate = useNavigate()
  const topo = useQuery({ queryKey: ['topology'], queryFn: getTopology, refetchInterval: 15000 })
  const ca = useQuery({ queryKey: ['ca'], queryFn: getCAInfo })

  const nodes = topo.data?.frps ?? []
  const hosts = topo.data?.hosts ?? []
  const onlineNodes = nodes.filter((n) => n.status === 'online').length
  const onlineHosts = hosts.filter((h) => h.status === 'online').length
  const totalConns = hosts.reduce((sum, h) => sum + (h.connections?.length ?? 0), 0)
  const totalProxies = hosts.reduce((sum, h) => sum + (h.connections ?? []).reduce((s, c) => s + (c.proxies?.length ?? 0), 0), 0)
  const activeConns = nodes.reduce((sum, n) => sum + (n.runtime?.active_connections ?? 0), 0)

  const caInfo = ca.data
  const caExpiryColor = !caInfo ? 'default' : caInfo.expired ? '#cf1322' : caInfo.days_remaining <= 60 ? '#d46b08' : '#3f8600'

  const frpsColumns = [
    { title: '名称', dataIndex: 'name', render: (v: string, r: FRPSNode) => <a onClick={() => navigate(`/frps/${r.uuid}`)}>{v}</a> },
    { title: '状态', dataIndex: 'status', render: (v: string) => <StatusBadge status={v} /> },
    { title: '公网 IP', dataIndex: 'public_ip', render: (v: string) => <span className="mono">{v || '-'}</span> },
    { title: '服务端口', dataIndex: 'bind_port' },
    { title: 'frp 版本', dataIndex: 'frp_version', render: (v: string) => <Tag>{v || '-'}</Tag> },
    { title: '系统/架构', render: (_: unknown, r: FRPSNode) => archLabel(r.runtime?.os, r.runtime?.arch) },
    { title: '运行时长', render: (_: unknown, r: FRPSNode) => formatUptime(r.runtime?.uptime_sec) },
    { title: '连接数', render: (_: unknown, r: FRPSNode) => r.runtime?.active_connections ?? 0 },
  ]

  const hostColumns = [
    { title: '名称', dataIndex: 'name', render: (v: string, r: FRPCHost) => <a onClick={() => navigate(`/frpc/${r.uuid}`)}>{v}</a> },
    { title: '状态', dataIndex: 'status', render: (v: string) => <StatusBadge status={v} /> },
    { title: '连接数', render: (_: unknown, r: FRPCHost) => r.connections?.length ?? 0 },
    { title: 'frp 版本', dataIndex: 'frp_version', render: (v: string) => <Tag>{v || '-'}</Tag> },
    { title: '系统/架构', render: (_: unknown, r: FRPCHost) => archLabel(r.runtime?.os, r.runtime?.arch) },
    { title: '运行时长', render: (_: unknown, r: FRPCHost) => formatUptime(r.runtime?.uptime_sec) },
  ]

  return (
    <>
      <Title level={3}>节点总览</Title>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="FRPS 节点" value={nodes.length} prefix={<CloudServerOutlined />} suffix={`/ 在线 ${onlineNodes}`} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="FRPC 主机" value={hosts.length} prefix={<ApiOutlined />} suffix={`/ 在线 ${onlineHosts}`} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="连接 / 映射" value={totalConns} suffix={`/ ${totalProxies} 映射`} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="活动连接数" value={activeConns} prefix={<ThunderboltOutlined />} />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col xs={24} lg={16}>
          <Card title="FRPS 节点运行环境" size="small" style={{ marginBottom: 16 }}>
            <Table rowKey="uuid" size="small" pagination={false} loading={topo.isLoading} dataSource={nodes} columns={frpsColumns} />
          </Card>
          <Card title="FRPC 主机运行环境" size="small">
            <Table rowKey="uuid" size="small" pagination={false} loading={topo.isLoading} dataSource={hosts} columns={hostColumns} />
          </Card>
        </Col>
        <Col xs={24} lg={8}>
          <Card
            title={<><SafetyCertificateOutlined /> CA 证书</>}
            size="small"
            extra={<a onClick={() => navigate('/settings')}>详情</a>}
          >
            {caInfo ? (
              <>
                <Statistic
                  title="剩余有效期"
                  value={caInfo.expired ? '已过期' : caInfo.days_remaining}
                  suffix={caInfo.expired ? '' : '天'}
                  valueStyle={{ color: caExpiryColor }}
                />
                <div style={{ marginTop: 12, fontSize: 13, lineHeight: 1.9 }}>
                  <div>主题：<span className="mono">{caInfo.subject}</span></div>
                  <div>算法：{caInfo.public_key_algorithm}{caInfo.key_bits ? ` ${caInfo.key_bits} bit` : ''}</div>
                  <div>过期：{caInfo.not_after.slice(0, 10)}</div>
                  <div style={{ wordBreak: 'break-all' }}>指纹：<span className="mono" style={{ fontSize: 11 }}>{caInfo.fingerprint_sha256}</span></div>
                </div>
              </>
            ) : (
              <Typography.Text type="secondary">加载中…</Typography.Text>
            )}
          </Card>
        </Col>
      </Row>
    </>
  )
}
