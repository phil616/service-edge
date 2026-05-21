import { Button, Card, Col, Divider, Form, Input, InputNumber, Row, Select, Space, Typography, message } from 'antd'
import { MinusCircleOutlined, PlusOutlined } from '@ant-design/icons'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createConnection, frpsPortAvailability, listFRPS } from '../api/client'
import { PROTOCOL_LABELS, nodeProtocols } from '../lib/transport'
import type { ProxyInput } from '../api/types'

const PROXY_TYPES = ['tcp', 'udp', 'http', 'https']

export default function ConnectionNew() {
  const { hostUuid = '' } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [form] = Form.useForm()
  const selectedFrps = Form.useWatch('frps_uuid', form)

  const { data: nodes } = useQuery({ queryKey: ['frps'], queryFn: listFRPS })
  const { data: ports } = useQuery({
    queryKey: ['frps-ports-avail', selectedFrps],
    queryFn: () => frpsPortAvailability(selectedFrps),
    enabled: !!selectedFrps,
  })

  const onlineNodes = (nodes ?? []).filter((n) => n.status === 'online')
  const selectedNode = (nodes ?? []).find((n) => n.uuid === selectedFrps)
  const availableProtocols = nodeProtocols(selectedNode)
  const usedPorts = ports?.used_ports ?? []
  const hostOccupied = ports?.host_occupied_ports ?? []

  const create = useMutation({
    mutationFn: (body: any) => createConnection(hostUuid, body),
    onSuccess: (conn) => {
      qc.invalidateQueries({ queryKey: ['frpc-host', hostUuid] })
      if (conn.proxies?.some((p) => p.inactive)) {
        message.warning('部分远程端口已被目标主机占用，对应映射未激活')
      }
      navigate(`/connections/${conn.uuid}`)
    },
  })

  const onFinish = (values: any) => {
    const proxies: ProxyInput[] = (values.proxies ?? []).map((p: any) => ({
      name: p.name,
      proxy_type: p.proxy_type,
      local_ip: p.local_ip || '127.0.0.1',
      local_port: p.local_port,
      remote_port: p.remote_port ?? null,
      custom_domains: p.custom_domains
        ? String(p.custom_domains).split(',').map((s: string) => s.trim()).filter(Boolean)
        : undefined,
      subdomain: p.subdomain || undefined,
    }))
    create.mutate({ name: values.name, frps_uuid: values.frps_uuid, protocol: values.protocol || 'tcp', proxies })
  }

  return (
    <Card title={<Typography.Title level={4} style={{ margin: 0 }}>新增连接</Typography.Title>}>
      <Form form={form} layout="vertical" style={{ maxWidth: 820 }} initialValues={{ protocol: 'tcp', proxies: [{ proxy_type: 'tcp', local_ip: '127.0.0.1' }] }} onFinish={onFinish}>
        <Row gutter={16}>
          <Col span={8}>
            <Form.Item name="name" label="连接名称" rules={[{ required: true }]}>
              <Input placeholder="例如 to-tokyo" />
            </Form.Item>
          </Col>
          <Col span={10}>
            <Form.Item name="frps_uuid" label="目标 FRPS 节点（仅在线）" rules={[{ required: true }]}>
              <Select
                placeholder={onlineNodes.length ? '选择节点' : '暂无在线节点'}
                options={onlineNodes.map((n) => ({ value: n.uuid, label: `${n.name} (${n.public_ip || n.uuid.slice(0, 8)})` }))}
              />
            </Form.Item>
          </Col>
          <Col span={6}>
            <Form.Item name="protocol" label="传输协议" extra={selectedFrps ? undefined : '请先选择节点'}>
              <Select disabled={!selectedFrps} options={availableProtocols.map((p) => ({ value: p, label: PROTOCOL_LABELS[p] }))} />
            </Form.Item>
          </Col>
        </Row>

        <Divider orientation="left">端口映射</Divider>
        <Form.List name="proxies">
          {(fields, { add, remove }) => (
            <>
              {fields.map(({ key, name, ...rest }) => (
                <ProxyRow key={key} name={name} rest={rest} form={form} usedPorts={usedPorts} hostOccupied={hostOccupied} onRemove={() => remove(name)} />
              ))}
              <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add({ proxy_type: 'tcp', local_ip: '127.0.0.1' })}>
                添加映射
              </Button>
            </>
          )}
        </Form.List>

        <Divider />
        <Button type="primary" htmlType="submit" loading={create.isPending}>
          创建连接
        </Button>
      </Form>
    </Card>
  )
}

function ProxyRow({ name, rest, form, usedPorts, hostOccupied, onRemove }: any) {
  const type = Form.useWatch(['proxies', name, 'proxy_type'], form)
  const isTcpUdp = type === 'tcp' || type === 'udp'

  return (
    <Space align="baseline" wrap style={{ display: 'flex', marginBottom: 8 }}>
      <Form.Item {...rest} name={[name, 'name']} rules={[{ required: true, message: '名称' }]}>
        <Input placeholder="映射名称" style={{ width: 130 }} />
      </Form.Item>
      <Form.Item {...rest} name={[name, 'proxy_type']} rules={[{ required: true }]}>
        <Select style={{ width: 90 }} options={PROXY_TYPES.map((t) => ({ value: t, label: t.toUpperCase() }))} />
      </Form.Item>
      <Form.Item {...rest} name={[name, 'local_ip']}>
        <Input placeholder="本地 IP" style={{ width: 120 }} />
      </Form.Item>
      <Form.Item {...rest} name={[name, 'local_port']} rules={[{ required: true, message: '本地端口' }]}>
        <InputNumber placeholder="本地端口" min={1} max={65535} style={{ width: 110 }} />
      </Form.Item>
      {isTcpUdp ? (
        <Form.Item
          {...rest}
          name={[name, 'remote_port']}
          rules={[
            { required: true, message: '远程端口' },
            {
              validator: (_, value) =>
                value && usedPorts.includes(Number(value))
                  ? Promise.reject(new Error('端口已被占用'))
                  : Promise.resolve(),
            },
          ]}
          extra={hostOccupied.length ? `主机外部占用：${hostOccupied.join(', ')}` : undefined}
        >
          <InputNumber placeholder="远程端口" min={1} max={65535} style={{ width: 110 }} />
        </Form.Item>
      ) : (
        <>
          <Form.Item {...rest} name={[name, 'custom_domains']}>
            <Input placeholder="自定义域名(逗号分隔)" style={{ width: 200 }} />
          </Form.Item>
          <Form.Item {...rest} name={[name, 'subdomain']}>
            <Input placeholder="子域名" style={{ width: 110 }} />
          </Form.Item>
        </>
      )}
      <MinusCircleOutlined onClick={onRemove} style={{ color: '#cf1322' }} />
    </Space>
  )
}
