import { Button, Card, Form, Input, InputNumber, Switch, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createFRPS } from '../api/client'

export default function FRPSNew() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [form] = Form.useForm()
  const dashboardEnabled = Form.useWatch('dashboard_enabled', form)

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
    create.mutate(body)
  }

  return (
    <Card title={<Typography.Title level={4} style={{ margin: 0 }}>创建 FRPS 节点</Typography.Title>}>
      <Form
        form={form}
        layout="vertical"
        style={{ maxWidth: 560 }}
        initialValues={{ bind_port: 7000, frp_version: 'v0.61.1', dashboard_enabled: false }}
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
        <Form.Item name="frp_version" label="FRP 版本">
          <Input placeholder="v0.61.1" />
        </Form.Item>
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
