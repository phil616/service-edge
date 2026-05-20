import { Card, Col, Descriptions, Row, Table, Tag, Typography } from 'antd'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { frpsInstallCommand, frpsPortUsage, getFRPS } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import InstallCommand from '../components/InstallCommand'
import HostRuntime from '../components/HostRuntime'
import CertDescriptions from '../components/CertDescriptions'
import type { PortUse } from '../api/types'

const KIND_LABEL: Record<string, { text: string; color: string }> = {
  bind: { text: '服务端口', color: 'blue' },
  dashboard: { text: 'Dashboard', color: 'purple' },
  proxy: { text: '映射', color: 'green' },
  host: { text: '主机占用(外部)', color: 'red' },
}

export default function FRPSDetail() {
  const { uuid = '' } = useParams()
  const { data } = useQuery({
    queryKey: ['frps', uuid],
    queryFn: () => getFRPS(uuid),
    refetchInterval: 10000,
  })
  const { data: ports } = useQuery({
    queryKey: ['frps-ports', uuid],
    queryFn: () => frpsPortUsage(uuid),
    refetchInterval: 10000,
  })

  const portColumns = [
    { title: '端口', dataIndex: 'port', render: (v: number) => <span className="mono">{v}</span>, sorter: (a: PortUse, b: PortUse) => a.port - b.port },
    { title: '用途', dataIndex: 'kind', render: (v: string) => { const k = KIND_LABEL[v] ?? { text: v, color: 'default' }; return <Tag color={k.color}>{k.text}</Tag> } },
    { title: '协议', dataIndex: 'proxy_type', render: (v?: string) => (v ? v.toUpperCase() : '-') },
    { title: '映射名称', dataIndex: 'proxy_name', render: (v?: string) => v || '-' },
    { title: '所属客户端', dataIndex: 'frpc_name', render: (v?: string) => v || '-' },
  ]

  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} lg={14}>
        <Card title={<Typography.Title level={4} style={{ margin: 0 }}>{data?.name ?? 'FRPS 节点'}</Typography.Title>} style={{ marginBottom: 16 }}>
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="UUID"><span className="mono">{data?.uuid}</span></Descriptions.Item>
            <Descriptions.Item label="状态">{data && <StatusBadge status={data.status} />}</Descriptions.Item>
            <Descriptions.Item label="服务端口">{data?.bind_port}</Descriptions.Item>
            <Descriptions.Item label="公网 IP">{data?.public_ip || '-'}</Descriptions.Item>
            <Descriptions.Item label="Dashboard 端口">{data?.dashboard_port || '未启用'}</Descriptions.Item>
            <Descriptions.Item label="FRP 版本"><Tag>{data?.frp_version}</Tag></Descriptions.Item>
            <Descriptions.Item label="配置版本">{data?.config_version}</Descriptions.Item>
            <Descriptions.Item label="最后心跳">
              {data?.last_heartbeat ? dayjs(data.last_heartbeat).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
          </Descriptions>
        </Card>

        <Card title="占用端口" size="small" style={{ marginBottom: 16 }}>
          <Table rowKey={(r) => `${r.kind}-${r.port}`} size="small" pagination={false} dataSource={ports ?? []} columns={portColumns} />
        </Card>

        <Card title="节点证书 (frps server)" size="small">
          <CertDescriptions info={data?.tls_cert_info} />
        </Card>
      </Col>

      <Col xs={24} lg={10}>
        <Card title="主机运行环境" style={{ marginBottom: 16 }}>
          <HostRuntime runtime={data?.runtime} />
        </Card>
        <Card title="安装命令">
          <InstallCommand generate={() => frpsInstallCommand(uuid)} />
        </Card>
      </Col>
    </Row>
  )
}
