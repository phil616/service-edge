import { Card, Col, Row, Statistic, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { listFRPC, listFRPS } from '../api/client'

export default function Dashboard() {
  const frps = useQuery({ queryKey: ['frps'], queryFn: listFRPS })
  const frpc = useQuery({ queryKey: ['frpc'], queryFn: listFRPC })

  const nodes = frps.data ?? []
  const clients = frpc.data ?? []
  const onlineNodes = nodes.filter((n) => n.status === 'online').length
  const onlineClients = clients.filter((c) => c.status === 'online').length

  return (
    <>
      <Typography.Title level={3}>节点总览</Typography.Title>
      <Row gutter={16}>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="FRPS 节点" value={nodes.length} suffix={`/ 在线 ${onlineNodes}`} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="FRPC 客户端" value={clients.length} suffix={`/ 在线 ${onlineClients}`} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="待部署节点" value={nodes.filter((n) => n.status === 'pending').length} />
          </Card>
        </Col>
        <Col xs={12} md={6}>
          <Card>
            <Statistic title="待部署客户端" value={clients.filter((c) => c.status === 'pending').length} />
          </Card>
        </Col>
      </Row>
    </>
  )
}
