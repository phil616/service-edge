import { useState } from 'react'
import { Button, Card, Col, Descriptions, Form, Input, InputNumber, Modal, Popconfirm, Row, Select, Space, Table, Tag, Typography } from 'antd'
import { EditOutlined, PlusOutlined } from '@ant-design/icons'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { addProxy, deleteProxy, frpcInstallCommand, getFRPC, getFRPS, updateFRPC, updateProxy } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import InstallCommand from '../components/InstallCommand'
import type { ProxyMapping } from '../api/types'

const PROXY_TYPES = ['tcp', 'udp', 'http', 'https']

// Stored custom_domains is a JSON array string; show it comma-separated for editing.
function domainsToText(raw?: string) {
  if (!raw) return ''
  try {
    return (JSON.parse(raw) as string[]).join(', ')
  } catch {
    return raw
  }
}

export default function FRPCDetail() {
  const { uuid = '' } = useParams()
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [editing, setEditing] = useState<ProxyMapping | null>(null)
  const [clientOpen, setClientOpen] = useState(false)
  const [form] = Form.useForm()
  const [clientForm] = Form.useForm()
  const proxyType = Form.useWatch('proxy_type', form)

  const { data } = useQuery({ queryKey: ['frpc', uuid], queryFn: () => getFRPC(uuid), refetchInterval: 10000 })
  const { data: node } = useQuery({
    queryKey: ['frps', data?.frps_uuid],
    queryFn: () => getFRPS(data!.frps_uuid),
    enabled: !!data?.frps_uuid,
  })

  const invalidate = () => qc.invalidateQueries({ queryKey: ['frpc', uuid] })
  const closeProxyModal = () => {
    setModalOpen(false)
    setEditing(null)
    form.resetFields()
  }
  const add = useMutation({
    mutationFn: (body: any) => addProxy(uuid, body),
    onSuccess: () => {
      invalidate()
      closeProxyModal()
    },
  })
  const update = useMutation({
    mutationFn: ({ id, body }: { id: number; body: any }) => updateProxy(id, body),
    onSuccess: () => {
      invalidate()
      closeProxyModal()
    },
  })
  const del = useMutation({ mutationFn: deleteProxy, onSuccess: invalidate })
  const updateClient = useMutation({
    mutationFn: (body: any) => updateFRPC(uuid, body),
    onSuccess: () => {
      invalidate()
      setClientOpen(false)
    },
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

  const openEditClient = () => {
    clientForm.setFieldsValue({ name: data?.name, frp_version: data?.frp_version })
    setClientOpen(true)
  }

  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '协议', dataIndex: 'proxy_type', render: (v: string) => <Tag>{v.toUpperCase()}</Tag> },
    { title: '本地', render: (_: unknown, r: ProxyMapping) => `${r.local_ip}:${r.local_port}` },
    { title: '访问地址', render: (_: unknown, r: ProxyMapping) => <span className="mono">{accessAddr(r)}</span> },
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
    if (editing) {
      update.mutate({ id: editing.id, body })
    } else {
      add.mutate(body)
    }
  }

  const isTcpUdp = proxyType === 'tcp' || proxyType === 'udp'

  return (
    <Row gutter={16}>
      <Col xs={24} lg={14}>
        <Card
          title={<Typography.Title level={4} style={{ margin: 0 }}>{data?.name ?? 'FRPC 客户端'}</Typography.Title>}
          extra={<Button icon={<EditOutlined />} onClick={openEditClient}>编辑</Button>}
          style={{ marginBottom: 16 }}
        >
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="UUID"><span className="mono">{data?.uuid}</span></Descriptions.Item>
            <Descriptions.Item label="状态">{data && <StatusBadge status={data.status} />}</Descriptions.Item>
            <Descriptions.Item label="目标 FRPS">{node?.name ?? data?.frps_uuid}</Descriptions.Item>
            <Descriptions.Item label="FRP 版本">{data?.frp_version}</Descriptions.Item>
            <Descriptions.Item label="配置版本">{data?.config_version}</Descriptions.Item>
            <Descriptions.Item label="最后心跳">
              {data?.last_heartbeat ? dayjs(data.last_heartbeat).format('YYYY-MM-DD HH:mm:ss') : '-'}
            </Descriptions.Item>
          </Descriptions>
        </Card>
        <Card
          title="端口映射"
          extra={<Button icon={<PlusOutlined />} onClick={openAdd}>新增映射</Button>}
        >
          <Table rowKey="id" dataSource={data?.proxies ?? []} columns={columns} pagination={false} />
        </Card>
      </Col>
      <Col xs={24} lg={10}>
        <Card title="安装命令">
          <InstallCommand generate={() => frpcInstallCommand(uuid)} />
        </Card>
      </Col>

      <Modal
        title={editing ? '编辑端口映射' : '新增端口映射'}
        open={modalOpen}
        onOk={() => form.submit()}
        confirmLoading={add.isPending || update.isPending}
        onCancel={closeProxyModal}
        forceRender
      >
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
            <Form.Item name="remote_port" label="远程端口" rules={[{ required: true }]}>
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

      <Modal
        title="编辑 FRPC 客户端"
        open={clientOpen}
        onOk={() => clientForm.submit()}
        confirmLoading={updateClient.isPending}
        onCancel={() => setClientOpen(false)}
        forceRender
      >
        <Form form={clientForm} layout="vertical" onFinish={(v) => updateClient.mutate(v)}>
          <Form.Item name="name" label="客户端名称" rules={[{ required: true }]}>
            <Input placeholder="例如 home-nas" />
          </Form.Item>
          <Form.Item name="frp_version" label="FRP 版本" extra="留空跟随目标节点版本">
            <Input placeholder="例如 0.61.0" />
          </Form.Item>
        </Form>
      </Modal>
    </Row>
  )
}
