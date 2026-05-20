import { Card, Col, Descriptions, Row, Typography } from 'antd'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { frpsInstallCommand, getFRPS } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import InstallCommand from '../components/InstallCommand'

export default function FRPSDetail() {
  const { uuid = '' } = useParams()
  const { data } = useQuery({
    queryKey: ['frps', uuid],
    queryFn: () => getFRPS(uuid),
    refetchInterval: 10000,
  })

  return (
    <Row gutter={16}>
      <Col xs={24} lg={14}>
        <Card title={<Typography.Title level={4} style={{ margin: 0 }}>{data?.name ?? 'FRPS 节点'}</Typography.Title>}>
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="UUID"><span className="mono">{data?.uuid}</span></Descriptions.Item>
            <Descriptions.Item label="状态">{data && <StatusBadge status={data.status} />}</Descriptions.Item>
            <Descriptions.Item label="服务端口">{data?.bind_port}</Descriptions.Item>
            <Descriptions.Item label="公网 IP">{data?.public_ip || '-'}</Descriptions.Item>
            <Descriptions.Item label="Dashboard 端口">{data?.dashboard_port || '未启用'}</Descriptions.Item>
            <Descriptions.Item label="FRP 版本">{data?.frp_version}</Descriptions.Item>
            <Descriptions.Item label="配置版本">{data?.config_version}</Descriptions.Item>
            <Descriptions.Item label="最后心跳">
              {data?.last_heartbeat ? dayjs(data.last_heartbeat).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
          </Descriptions>
        </Card>
      </Col>
      <Col xs={24} lg={10}>
        <Card title="安装命令">
          <InstallCommand generate={() => frpsInstallCommand(uuid)} />
        </Card>
      </Col>
    </Row>
  )
}
