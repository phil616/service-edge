import { useEffect, useMemo } from 'react'
import { Button, Card, Form, Input, Typography } from 'antd'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFRPCHost, listFRPDists } from '../api/client'
import { latestFrpVersion } from '../lib/frp'

export default function FRPCNew() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [form] = Form.useForm()

  // Default the frp version to the latest release present in dist management.
  const { data: dists } = useQuery({ queryKey: ['frp-dists'], queryFn: listFRPDists })
  const latest = useMemo(() => latestFrpVersion(dists ?? []), [dists])
  useEffect(() => {
    if (latest && !form.getFieldValue('frp_version')) form.setFieldValue('frp_version', latest)
  }, [latest, form])

  const create = useMutation({
    mutationFn: createFRPCHost,
    onSuccess: (host) => {
      qc.invalidateQueries({ queryKey: ['frpc-hosts'] })
      navigate(`/frpc/${host.uuid}`)
    },
  })

  const onFinish = (values: Record<string, unknown>) => {
    create.mutate({ name: values.name, frp_version: values.frp_version || '' })
  }

  return (
    <Card title={<Typography.Title level={4} style={{ margin: 0 }}>创建 FRPC 主机</Typography.Title>}>
      <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
        主机是一台运行 frpc Agent 的机器，安装一次后可在其下添加多个连接（每个连接连往一个 FRPS）。创建后在主机详情页生成安装命令并部署 Agent。
      </Typography.Paragraph>
      <Form form={form} layout="vertical" style={{ maxWidth: 520 }} onFinish={onFinish}>
        <Form.Item name="name" label="主机名称" rules={[{ required: true }]}>
          <Input placeholder="例如 home-nas" />
        </Form.Item>
        <Form.Item name="frp_version" label="FRP 版本" extra={latest ? `该主机上 frpc 二进制的版本，所有连接共用。默认使用发行版管理中的最新版本 ${latest}` : '该主机上 frpc 二进制的版本，所有连接共用'}>
          <Input placeholder={latest || 'v0.61.1'} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={create.isPending}>
          创建
        </Button>
      </Form>
    </Card>
  )
}
