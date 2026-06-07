import { useEffect, useMemo } from 'react'
import { Button, Card, Divider, Form, Input, InputNumber, Switch, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFRPS, listFRPDists } from '../api/client'
import { latestFrpVersion } from '../lib/frp'
import DNSResolver from '../components/DNSResolver'

export default function FRPSNew() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [form] = Form.useForm()
  const dashboardEnabled = Form.useWatch('dashboard_enabled', form)
  const kcpEnabled = Form.useWatch('kcp_enabled', form)
  const quicEnabled = Form.useWatch('quic_enabled', form)

  // Default the frp version to the latest release present in dist management.
  const { data: dists } = useQuery({ queryKey: ['frp-dists'], queryFn: listFRPDists })
  const latest = useMemo(() => latestFrpVersion(dists ?? []), [dists])
  useEffect(() => {
    if (latest && !form.getFieldValue('frp_version')) form.setFieldValue('frp_version', latest)
  }, [latest, form])

  const create = useMutation({
    mutationFn: createFRPS,
    onSuccess: (node) => {
      qc.invalidateQueries({ queryKey: ['frps'] })
      navigate(`/frps/${node.uuid}`)
    },
  })

  const onFinish = (values: Record<string, unknown>) => {
    const body: Record<string, unknown> = {
      name: values.name,
      bind_port: values.bind_port,
      public_ip: values.public_ip || '',
      frp_version: values.frp_version || '',
    }
    if (values.dashboard_enabled) {
      body.dashboard_port = values.dashboard_port
      body.dashboard_user = values.dashboard_user
      body.dashboard_pwd = values.dashboard_pwd
    }
    if (values.kcp_enabled) body.kcp_bind_port = values.kcp_bind_port
    if (values.quic_enabled) body.quic_bind_port = values.quic_bind_port
    create.mutate(body)
  }

  return (
    <Card title={<Typography.Title level={4} style={{ margin: 0 }}>创建 FRPS 节点</Typography.Title>}>
      <Form
        form={form}
        layout="vertical"
        style={{ maxWidth: 560 }}
        initialValues={{ bind_port: 7000, dashboard_enabled: false }}
        onFinish={onFinish}
      >
        <Form.Item name="name" label="边缘节点名称" rules={[{ required: true }]}>
          <Input placeholder="例如 edge-tokyo" />
        </Form.Item>
        <Form.Item name="bind_port" label="服务端口" rules={[{ required: true }]} extra="frpc 连接的 TCP 端口，需在防火墙/安全组开放">
          <InputNumber min={1} max={65535} style={{ width: '100%' }} />
        </Form.Item>
        <Form.Item name="public_ip" label="公网 IP（选填）" extra="用于生成 frpc 的连接地址与访问提示">
          <Input placeholder="例如 203.0.113.10" />
        </Form.Item>
        <DNSResolver onResolved={(ip) => form.setFieldValue('public_ip', ip)} />
        <Form.Item name="frp_version" label="FRP 版本" extra={latest ? `默认使用发行版管理中的最新版本 ${latest}` : '发行版管理中暂无可用版本，将使用内置默认版本'}>
          <Input placeholder={latest || 'v0.61.1'} />
        </Form.Item>

        <Divider orientation="left" plain>传输协议</Divider>
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          TCP / WebSocket / WSS 复用上面的服务端口，无需额外配置。如需更高抗丢包/低延迟，可启用 KCP 或 QUIC（基于 UDP，需在防火墙/安全组开放对应 UDP 端口）。
        </Typography.Paragraph>
        <Form.Item name="kcp_enabled" label="启用 KCP（UDP）" valuePropName="checked">
          <Switch />
        </Form.Item>
        {kcpEnabled && (
          <Form.Item name="kcp_bind_port" label="KCP 端口（UDP）" rules={[{ required: true }]} extra="可与服务端口相同">
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
        )}
        <Form.Item name="quic_enabled" label="启用 QUIC（UDP）" valuePropName="checked">
          <Switch />
        </Form.Item>
        {quicEnabled && (
          <Form.Item name="quic_bind_port" label="QUIC 端口（UDP）" rules={[{ required: true }]} extra="必须与服务端口不同">
            <InputNumber min={1} max={65535} style={{ width: '100%' }} />
          </Form.Item>
        )}

        <Divider orientation="left" plain>Dashboard</Divider>
        <Form.Item name="dashboard_enabled" label="启用 Dashboard" valuePropName="checked">
          <Switch />
        </Form.Item>
        {dashboardEnabled && (
          <>
            <Form.Item name="dashboard_port" label="Web 端口" rules={[{ required: true }]}>
              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
            </Form.Item>
            <Form.Item name="dashboard_user" label="Web 用户名" rules={[{ required: true }]}>
              <Input />
            </Form.Item>
            <Form.Item name="dashboard_pwd" label="Web 密码" rules={[{ required: true }]}>
              <Input.Password />
            </Form.Item>
          </>
        )}
        <Button type="primary" htmlType="submit" loading={create.isPending}>
          创建
        </Button>
      </Form>
    </Card>
  )
}
