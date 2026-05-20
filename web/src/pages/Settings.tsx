import { useEffect } from 'react'
import { Button, Card, Descriptions, Form, Input, Space, Typography, message } from 'antd'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { fetchMe, getCAInfo, getSettings, updateSettings } from '../api/client'
import CertDescriptions from '../components/CertDescriptions'

// GitHub Release 作为 Agent 下载源：安装脚本会在基址后追加 _linux_<arch>，
// 恰好对应 Release 产物 agent_linux_amd64 / agent_linux_arm64。
const GITHUB_REPO = 'https://github.com/phil616/service-edge'
const GITHUB_LATEST_AGENT_BASE = `${GITHUB_REPO}/releases/latest/download/agent`

// ReleaseHint renders the per-field suggestion: an example, the copyable GitHub
// Release base, and a one-click button to fill the field.
function ReleaseHint({ onFill }: { onFill: () => void }) {
  return (
    <span>
      例如 <Typography.Text code style={{ fontSize: 12 }}>https://cdn.example.com/service-edge/agent</Typography.Text>。
      也可直接使用 GitHub Release（始终指向最新版，追加 <Typography.Text code style={{ fontSize: 12 }}>_linux_&lt;arch&gt;</Typography.Text> 即得 <Typography.Text code style={{ fontSize: 12 }}>agent_linux_amd64/arm64</Typography.Text> 产物）：
      <br />
      <Typography.Text code copyable={{ text: GITHUB_LATEST_AGENT_BASE }} style={{ fontSize: 12 }}>
        {GITHUB_LATEST_AGENT_BASE}
      </Typography.Text>
      <Button type="link" size="small" style={{ paddingInline: 4 }} onClick={onFill}>
        填入此地址
      </Button>
    </span>
  )
}

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
          推荐直接使用 <Typography.Text strong>GitHub Release</Typography.Text> 产物作为下载源（见下方提示）。
          留空则回退到控制平面默认地址：<Typography.Text code>{defaultBase}</Typography.Text>。
        </Typography.Paragraph>
        <Form form={form} layout="vertical" onFinish={(v) => save.mutate(v)} style={{ maxWidth: 640 }}>
          <Form.Item
            name="agent_download_url_frps"
            label="FRPS Agent 下载基址"
            extra={<ReleaseHint onFill={() => form.setFieldValue('agent_download_url_frps', GITHUB_LATEST_AGENT_BASE)} />}
          >
            <Input allowClear placeholder={`留空使用默认：${defaultBase}`} />
          </Form.Item>
          <Form.Item
            name="agent_download_url_frpc"
            label="FRPC Agent 下载基址"
            extra={<ReleaseHint onFill={() => form.setFieldValue('agent_download_url_frpc', GITHUB_LATEST_AGENT_BASE)} />}
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
