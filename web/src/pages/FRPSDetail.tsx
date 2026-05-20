import { useState } from 'react'
import { Alert, Button, Card, Col, Descriptions, Form, Input, InputNumber, Modal, Row, Space, Table, Tag, Typography } from 'antd'
import { EditOutlined } from '@ant-design/icons'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { frpsInstallCommand, frpsPortUsage, getFRPS, updateFRPS } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import InstallCommand from '../components/InstallCommand'
import HostRuntime from '../components/HostRuntime'
import CertDescriptions from '../components/CertDescriptions'
import AgentSyncInfo from '../components/AgentSyncInfo'
import type { PortUse } from '../api/types'

const KIND_LABEL: Record<string, { text: string; color: string }> = {
  bind: { text: '服务端口', color: 'blue' },
  dashboard: { text: 'Dashboard', color: 'purple' },
  proxy: { text: '映射', color: 'green' },
  host: { text: '主机占用(外部)', color: 'red' },
}

export default function FRPSDetail() {
  const { uuid = '' } = useParams()
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [form] = Form.useForm()

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

  const update = useMutation({
    mutationFn: (body: any) => updateFRPS(uuid, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['frps', uuid] })
      qc.invalidateQueries({ queryKey: ['frps'] })
      qc.invalidateQueries({ queryKey: ['topology'] })
      setEditOpen(false)
    },
  })

  const openEdit = () => {
    form.setFieldsValue({
      name: data?.name,
      public_ip: data?.public_ip || '',
      bind_port: data?.bind_port,
      dashboard_port: data?.dashboard_port ?? undefined,
      dashboard_user: data?.dashboard_user || '',
      frp_version: data?.frp_version || '',
    })
    setEditOpen(true)
  }

  const onSave = (v: any) => {
    update.mutate({
      name: v.name,
      public_ip: v.public_ip ?? '',
      bind_port: v.bind_port,
      dashboard_port: v.dashboard_port ?? null,
      dashboard_user: v.dashboard_user || '',
      dashboard_pwd: v.dashboard_pwd || undefined,
      frp_version: v.frp_version || '',
    })
  }

  const portColumns = [
    { title: '端口', dataIndex: 'port', render: (v: number) => <span className="mono">{v}</span>, sorter: (a: PortUse, b: PortUse) => a.port - b.port },
    { title: '用途', dataIndex: 'kind', render: (v: string) => { const k = KIND_LABEL[v] ?? { text: v, color: 'default' }; return <Tag color={k.color}>{k.text}</Tag> } },
    { title: '协议', dataIndex: 'proxy_type', render: (v?: string) => (v ? v.toUpperCase() : '-') },
    { title: '映射名称', dataIndex: 'proxy_name', render: (v?: string) => v || '-' },
    { title: '所属客户端', dataIndex: 'frpc_name', render: (v?: string) => v || '-' },
  ]

  const noPublicIP = data != null && !data.public_ip

  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} lg={14}>
        <Card
          title={<Typography.Title level={4} style={{ margin: 0 }}>{data?.name ?? 'FRPS 节点'}</Typography.Title>}
          extra={<Button icon={<EditOutlined />} onClick={openEdit}>编辑</Button>}
          style={{ marginBottom: 16 }}
        >
          {noPublicIP && (
            <Alert
              type="warning"
              showIcon
              style={{ marginBottom: 12 }}
              message="未设置公网 IP"
              description="frpc 通过公网 IP 连接此节点。未设置时连接地址为占位符，frpc 将无法连通。请填写公网 IP，或等待该节点的 Agent 上线后自动识别。"
            />
          )}
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="UUID"><span className="mono">{data?.uuid}</span></Descriptions.Item>
            <Descriptions.Item label="状态">{data && <StatusBadge status={data.status} />}</Descriptions.Item>
            <Descriptions.Item label="服务端口">{data?.bind_port}</Descriptions.Item>
            <Descriptions.Item label="公网 IP">
              {data?.public_ip ? <span className="mono">{data.public_ip}</span> : <Typography.Text type="warning">未设置</Typography.Text>}
            </Descriptions.Item>
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
        <Card style={{ marginBottom: 16 }}>
          <AgentSyncInfo lastHeartbeat={data?.last_heartbeat} reportedAt={data?.runtime?.reported_at} status={data?.status} />
        </Card>
        <Card title="主机运行环境" style={{ marginBottom: 16 }}>
          <HostRuntime runtime={data?.runtime} />
        </Card>
        <Card title="安装命令">
          <InstallCommand generate={() => frpsInstallCommand(uuid)} />
        </Card>
      </Col>

      <Modal
        title="编辑 FRPS 节点"
        open={editOpen}
        onOk={() => form.submit()}
        confirmLoading={update.isPending}
        onCancel={() => setEditOpen(false)}
        forceRender
      >
        <Form form={form} layout="vertical" onFinish={onSave}>
          <Form.Item name="name" label="节点名称" rules={[{ required: true }]}>
            <Input placeholder="例如 edge-tokyo" />
          </Form.Item>
          <Form.Item name="public_ip" label="公网 IP" extra="frpc 连接此地址；留空将由 Agent 上线后自动识别">
            <Input allowClear placeholder="例如 203.0.113.10" />
          </Form.Item>
          <Form.Item name="bind_port" label="服务端口 (bindPort)" rules={[{ required: true }]}>
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
          <Space style={{ display: 'flex' }} align="baseline">
            <Form.Item name="dashboard_port" label="Dashboard 端口（选填）">
              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="dashboard_user" label="Dashboard 用户">
              <Input />
            </Form.Item>
            <Form.Item name="dashboard_pwd" label="Dashboard 密码" extra="留空不修改">
              <Input.Password />
            </Form.Item>
          </Space>
          <Form.Item name="frp_version" label="FRP 版本">
            <Input placeholder="例如 0.61.1" />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
