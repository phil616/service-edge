import { useState } from 'react'
import { Button, Card, Col, Descriptions, Form, Input, Modal, Popconfirm, Row, Space, Table, Tag, Typography } from 'antd'
import { EditOutlined, PlusOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { deleteConnection, frpcHostInstallCommand, getFRPCHost, listFRPS, updateFRPCHost } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import InstallCommand from '../components/InstallCommand'
import HostRuntime from '../components/HostRuntime'
import AgentSyncInfo from '../components/AgentSyncInfo'
import { protocolLabel } from '../lib/transport'
import type { FRPCConnection } from '../api/types'

export default function FRPCDetail() {
  const { uuid = '' } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [form] = Form.useForm()

  const { data } = useQuery({ queryKey: ['frpc-host', uuid], queryFn: () => getFRPCHost(uuid), refetchInterval: 10000 })
  const { data: nodes } = useQuery({ queryKey: ['frps'], queryFn: listFRPS })
  const nodeName = (u: string) => nodes?.find((n) => n.uuid === u)?.name ?? u.slice(0, 8)

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['frpc-host', uuid] })
    qc.invalidateQueries({ queryKey: ['frpc-hosts'] })
    qc.invalidateQueries({ queryKey: ['topology'] })
  }
  const update = useMutation({
    mutationFn: (body: any) => updateFRPCHost(uuid, body),
    onSuccess: () => {
      invalidate()
      setEditOpen(false)
    },
  })
  const delConn = useMutation({ mutationFn: deleteConnection, onSuccess: invalidate })

  const openEdit = () => {
    form.setFieldsValue({ name: data?.name, frp_version: data?.frp_version || '' })
    setEditOpen(true)
  }

  const connColumns = [
    { title: '连接名称', dataIndex: 'name', render: (v: string, r: FRPCConnection) => <a onClick={() => navigate(`/connections/${r.uuid}`)}>{v}</a> },
    { title: '目标 FRPS', dataIndex: 'frps_uuid', render: (v: string) => nodeName(v) },
    { title: '传输', dataIndex: 'protocol', render: (v?: string) => <Tag color="geekblue">{protocolLabel(v)}</Tag> },
    { title: '映射数', render: (_: unknown, r: FRPCConnection) => r.proxies?.length ?? 0 },
    { title: '状态', dataIndex: 'status', render: (v: string) => <StatusBadge status={v} /> },
    {
      title: '操作',
      render: (_: unknown, r: FRPCConnection) => (
        <Space>
          <a onClick={() => navigate(`/connections/${r.uuid}`)}>详情</a>
          <Popconfirm title="删除该连接？" onConfirm={() => delConn.mutate(r.uuid)}>
            <a style={{ color: '#cf1322' }}>删除</a>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} lg={14}>
        <Card
          title={<Typography.Title level={4} style={{ margin: 0 }}>{data?.name ?? 'FRPC 主机'}</Typography.Title>}
          extra={<Button icon={<EditOutlined />} onClick={openEdit}>编辑</Button>}
          style={{ marginBottom: 16 }}
        >
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="UUID"><span className="mono">{data?.uuid}</span></Descriptions.Item>
            <Descriptions.Item label="状态">{data && <StatusBadge status={data.status} />}</Descriptions.Item>
            <Descriptions.Item label="FRP 版本"><Tag>{data?.frp_version}</Tag></Descriptions.Item>
            <Descriptions.Item label="配置版本">{data?.config_version}</Descriptions.Item>
            <Descriptions.Item label="最后心跳">
              {data?.last_heartbeat ? dayjs(data.last_heartbeat).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
          </Descriptions>
        </Card>

        <Card
          title="连接（每个连往一个 FRPS）"
          extra={<Button icon={<PlusOutlined />} onClick={() => navigate(`/frpc/${uuid}/connections/new`)}>新增连接</Button>}
        >
          <Table rowKey="uuid" pagination={false} dataSource={data?.connections ?? []} columns={connColumns} />
        </Card>
      </Col>

      <Col xs={24} lg={10}>
        <Card style={{ marginBottom: 16 }}>
          <AgentSyncInfo lastHeartbeat={data?.last_heartbeat} reportedAt={data?.runtime?.reported_at} status={data?.status} />
        </Card>
        <Card title="主机运行环境" style={{ marginBottom: 16 }}>
          <HostRuntime runtime={data?.runtime} />
        </Card>
        <Card title="安装命令（在该主机执行一次）">
          <InstallCommand generate={() => frpcHostInstallCommand(uuid)} />
        </Card>
      </Col>

      <Modal title="编辑主机" open={editOpen} onOk={() => form.submit()} confirmLoading={update.isPending} onCancel={() => setEditOpen(false)} forceRender>
        <Form form={form} layout="vertical" onFinish={(v) => update.mutate(v)}>
          <Form.Item name="name" label="主机名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="frp_version" label="FRP 版本" extra="修改后所有连接将按新版本重新下发">
            <Input placeholder="v0.61.1" />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
