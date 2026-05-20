import { useEffect } from 'react'
import { Button, Card, Descriptions, Form, Input, Space, Typography, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchMe, getCAInfo, getSettings, updateSettings } from '../api/client'
import CertDescriptions from '../components/CertDescriptions'

export default function Settings() {
  const qc = useQueryClient()
  const { data } = useQuery({ queryKey: ['me'], queryFn: fetchMe })
  const { data: ca } = useQuery({ queryKey: ['ca'], queryFn: getCAInfo })
  const { data: settings } = useQuery({ queryKey: ['settings'], queryFn: getSettings })

  const [form] = Form.useForm()
  useEffect(() => {
    if (settings) {
      form.setFieldsValue({
        agent_download_url_frps: settings.agent_download_url_frps,
        agent_download_url_frpc: settings.agent_download_url_frpc,
      })
    }
  }, [settings, form])

  const save = useMutation({
    mutationFn: updateSettings,
    onSuccess: () => {
      message.success('已保存 Agent 下载设置')
      qc.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const defaultBase = settings?.control_plane_base || '（控制平面默认地址）'

  return (
    <Space direction="vertical" size={16} style={{ width: '100%', maxWidth: 900 }}>
      <Card title={<Typography.Title level={4} style={{ margin: 0 }}>系统设置</Typography.Title>}>
        <Descriptions column={1} bordered size="small">
          <Descriptions.Item label="当前用户">{data?.username}</Descriptions.Item>
          <Descriptions.Item label="用户 ID">{data?.id}</Descriptions.Item>
        </Descriptions>
        <Typography.Paragraph type="secondary" style={{ marginTop: 16 }}>
          前后端分离部署：前端 edge.dreamreflex.com，后端 edge-api.dreamreflex.com。认证使用 Bearer JWT，CORS 白名单限定前端域名。
        </Typography.Paragraph>
      </Card>

      <Card title="Agent 下载设置">
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          frps 与 frpc 的 Agent 可分别配置下载基址（可指向 CDN 加速、节省控制平面带宽）。
          填写的是<strong>基址</strong>，安装脚本会自动追加 <Typography.Text code>_linux_&lt;arch&gt;</Typography.Text> 后缀。
          留空则回退到控制平面默认地址：<Typography.Text code>{defaultBase}</Typography.Text>。
        </Typography.Paragraph>
        <Form form={form} layout="vertical" onFinish={(v) => save.mutate(v)} style={{ maxWidth: 640 }}>
          <Form.Item
            name="agent_download_url_frps"
            label="FRPS Agent 下载基址"
            extra="例如 https://cdn.example.com/service-edge/agent"
          >
            <Input allowClear placeholder={`留空使用默认：${defaultBase}`} />
          </Form.Item>
          <Form.Item
            name="agent_download_url_frpc"
            label="FRPC Agent 下载基址"
            extra="例如 https://cdn.example.com/service-edge/agent"
          >
            <Input allowClear placeholder={`留空使用默认：${defaultBase}`} />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={save.isPending}>
            保存
          </Button>
        </Form>
      </Card>

      <Card title="CA 证书详情">
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          控制平面使用此 CA 为每个 frps/frpc 签发短期叶子证书，frp 双向 TLS 校验均信任该 CA。
        </Typography.Paragraph>
        <CertDescriptions info={ca} />
      </Card>
    </Space>
  )
}
