import { useState } from 'react'
import { Button, Card, Col, Descriptions, Form, Input, InputNumber, Modal, Popconfirm, Row, Select, Space, Table, Tag, Tooltip, Typography, message } from 'antd'
import { EditOutlined, PlusOutlined } from '@ant-design/icons'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { addProxy, deleteProxy, frpsPortAvailability, getConnection, getFRPS, updateConnection, updateProxy } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import CertDescriptions from '../components/CertDescriptions'
import { PROTOCOL_LABELS, nodeProtocols } from '../lib/transport'
import type { ProxyMapping } from '../api/types'

const PROXY_TYPES = ['tcp', 'udp', 'http', 'https']

function domainsToText(raw?: string) {
  if (!raw) return ''
  try {
    return (JSON.parse(raw) as string[]).join(', ')
  } catch {
    return raw
  }
}

export default function ConnectionDetail() {
  const { uuid = '' } = useParams()
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ProxyMapping | null>(null)
  const [connOpen, setConnOpen] = useState(false)
  const [form] = Form.useForm()
  const [connForm] = Form.useForm()
  const proxyType = Form.useWatch('proxy_type', form)

  const { data } = useQuery({ queryKey: ['conn', uuid], queryFn: () => getConnection(uuid), refetchInterval: 10000 })
  const { data: node } = useQuery({
    queryKey: ['frps', data?.frps_uuid],
    queryFn: () => getFRPS(data!.frps_uuid),
    enabled: !!data?.frps_uuid,
  })
  const { data: portAvail } = useQuery({
    queryKey: ['frps-ports-avail', data?.frps_uuid],
    queryFn: () => frpsPortAvailability(data!.frps_uuid),
    enabled: !!data?.frps_uuid,
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['conn', uuid] })
  const closeProxyModal = () => {
    setModalOpen(false)
    setEditing(null)
    form.resetFields()
  }
  const warnIfInactive = (row?: ProxyMapping) => {
    if (row?.inactive) message.warning(row.inactive_reason || '端口被占用，映射未激活')
  }
  const add = useMutation({
    mutationFn: (body: any) => addProxy(uuid, body),
    onSuccess: (row) => { invalidate(); closeProxyModal(); warnIfInactive(row) },
  })
  const update = useMutation({
    mutationFn: ({ id, body }: { id: number; body: any }) => updateProxy(id, body),
    onSuccess: (row) => { invalidate(); closeProxyModal(); warnIfInactive(row) },
  })
  const del = useMutation({ mutationFn: deleteProxy, onSuccess: invalidate })
  const updateConn = useMutation({
    mutationFn: (body: any) => updateConnection(uuid, body),
    onSuccess: () => { invalidate(); setConnOpen(false) },
  })

  const accessAddr = (p: ProxyMapping) => {
    if ((p.proxy_type === 'tcp' || p.proxy_type === 'udp') && p.remote_port) {
      return `${node?.public_ip || '<公网IP>'}:${p.remote_port}`
    }
    if (p.custom_domains) {
      try {
        return (JSON.parse(p.custom_domains) as string[]).join(', ')
      } catch {
        return p.custom_domains
      }
    }
    return p.subdomain || '-'
  }

  const openAdd = () => {
    setEditing(null)
    form.resetFields()
    form.setFieldsValue({ proxy_type: 'tcp', local_ip: '127.0.0.1' })
    setModalOpen(true)
  }
  const openEdit = (p: ProxyMapping) => {
    setEditing(p)
    form.setFieldsValue({
      name: p.name,
      proxy_type: p.proxy_type,
      local_ip: p.local_ip,
      local_port: p.local_port,
      remote_port: p.remote_port ?? undefined,
      custom_domains: domainsToText(p.custom_domains),
      subdomain: p.subdomain || undefined,
    })
    setModalOpen(true)
  }
  const openEditConn = () => {
    connForm.setFieldsValue({ name: data?.name, protocol: data?.protocol || 'tcp' })
    setConnOpen(true)
  }

  const usedPorts = (portAvail?.used_ports ?? []).filter((p) => p !== editing?.remote_port)
  const hostOccupied = portAvail?.host_occupied_ports ?? []

  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '协议', dataIndex: 'proxy_type', render: (v: string) => <Tag>{v.toUpperCase()}</Tag> },
    { title: '本地', render: (_: unknown, r: ProxyMapping) => `${r.local_ip}:${r.local_port}` },
    { title: '访问地址', render: (_: unknown, r: ProxyMapping) => <span className="mono">{accessAddr(r)}</span> },
    {
      title: '状态',
      render: (_: unknown, r: ProxyMapping) =>
        r.inactive ? (
          <Tooltip title={r.inactive_reason}><Tag color="error">未激活</Tag></Tooltip>
        ) : (
          <Tag color="success">已激活</Tag>
        ),
    },
    {
      title: '操作',
      render: (_: unknown, r: ProxyMapping) => (
        <Space>
          <a onClick={() => openEdit(r)}>编辑</a>
          <Popconfirm title="删除该映射？" onConfirm={() => del.mutate(r.id)}>
            <a style={{ color: '#cf1322' }}>删除</a>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  const onSubmitProxy = (values: any) => {
    const body = {
      name: values.name,
      proxy_type: values.proxy_type,
      local_ip: values.local_ip || '127.0.0.1',
      local_port: values.local_port,
      remote_port: values.remote_port ?? null,
      custom_domains: values.custom_domains
        ? String(values.custom_domains).split(',').map((s: string) => s.trim()).filter(Boolean)
        : undefined,
      subdomain: values.subdomain || undefined,
    }
    editing ? update.mutate({ id: editing.id, body }) : add.mutate(body)
  }

  const isTcpUdp = proxyType === 'tcp' || proxyType === 'udp'

  return (
    <Row gutter={[16, 16]}>
      <Col xs={24} lg={14}>
        <Card
          title={<Typography.Title level={4} style={{ margin: 0 }}>{data?.name ?? 'FRPC 连接'}</Typography.Title>}
          extra={<Button icon={<EditOutlined />} onClick={openEditConn}>编辑</Button>}
          style={{ marginBottom: 16 }}
        >
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="UUID"><span className="mono">{data?.uuid}</span></Descriptions.Item>
            <Descriptions.Item label="所属主机">{data && <Link to={`/frpc/${data.host_uuid}`}>{data.host_uuid.slice(0, 8)}</Link>}</Descriptions.Item>
            <Descriptions.Item label="状态">{data && <StatusBadge status={data.status} />}</Descriptions.Item>
            <Descriptions.Item label="目标 FRPS">{node?.name ?? data?.frps_uuid}</Descriptions.Item>
            <Descriptions.Item label="传输协议"><Tag color="geekblue">{PROTOCOL_LABELS[data?.protocol ?? 'tcp']}</Tag></Descriptions.Item>
            <Descriptions.Item label="本地 Admin 端口"><span className="mono">{data?.admin_port}</span></Descriptions.Item>
            <Descriptions.Item label="配置版本">{data?.config_version}</Descriptions.Item>
          </Descriptions>
        </Card>
        <Card title="端口映射" extra={<Button icon={<PlusOutlined />} onClick={openAdd}>新增映射</Button>} style={{ marginBottom: 16 }}>
          <Table rowKey="id" dataSource={data?.proxies ?? []} columns={columns} pagination={false} />
        </Card>
        <Card title="连接证书 (frpc client)" size="small">
          <CertDescriptions info={data?.tls_cert_info} />
        </Card>
      </Col>

      <Modal title={editing ? '编辑端口映射' : '新增端口映射'} open={modalOpen} onOk={() => form.submit()} confirmLoading={add.isPending || update.isPending} onCancel={closeProxyModal} forceRender>
        <Form form={form} layout="vertical" initialValues={{ proxy_type: 'tcp', local_ip: '127.0.0.1' }} onFinish={onSubmitProxy}>
          <Form.Item name="name" label="映射名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="proxy_type" label="协议类型" rules={[{ required: true }]}>
            <Select options={PROXY_TYPES.map((t) => ({ value: t, label: t.toUpperCase() }))} />
          </Form.Item>
          <Space>
            <Form.Item name="local_ip" label="本地 IP">
              <Input />
            </Form.Item>
            <Form.Item name="local_port" label="本地端口" rules={[{ required: true }]}>
              <InputNumber min={1} max={65535} />
            </Form.Item>
          </Space>
          {isTcpUdp ? (
            <Form.Item
              name="remote_port"
              label="远程端口"
              rules={[
                { required: true },
                { validator: (_, value) => (value && usedPorts.includes(Number(value)) ? Promise.reject(new Error('该端口已被其他映射占用')) : Promise.resolve()) },
              ]}
              extra={hostOccupied.length ? `目标主机被外部进程占用的端口（填入将不会激活）：${hostOccupied.join(', ')}` : undefined}
            >
              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
            </Form.Item>
          ) : (
            <>
              <Form.Item name="custom_domains" label="自定义域名（逗号分隔）">
                <Input placeholder="a.example.com, b.example.com" />
              </Form.Item>
              <Form.Item name="subdomain" label="子域名">
                <Input />
              </Form.Item>
            </>
          )}
        </Form>
      </Modal>

      <Modal title="编辑连接" open={connOpen} onOk={() => connForm.submit()} confirmLoading={updateConn.isPending} onCancel={() => setConnOpen(false)} forceRender>
        <Form form={connForm} layout="vertical" onFinish={(v) => updateConn.mutate(v)}>
          <Form.Item name="name" label="连接名称" rules={[{ required: true }]}>
            <Input />
          </Form.Item>
          <Form.Item name="protocol" label="传输协议" extra="KCP / QUIC 需目标节点已启用对应 UDP 端口">
            <Select options={nodeProtocols(node).map((p) => ({ value: p, label: PROTOCOL_LABELS[p] }))} />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
