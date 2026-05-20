import { Card, Descriptions, Space, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { fetchMe, getCAInfo } from '../api/client'
import CertDescriptions from '../components/CertDescriptions'

export default function Settings() {
  const { data } = useQuery({ queryKey: ['me'], queryFn: fetchMe })
  const { data: ca } = useQuery({ queryKey: ['ca'], queryFn: getCAInfo })

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

      <Card title="CA 证书详情">
        <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
          控制平面使用此 CA 为每个 frps/frpc 签发短期叶子证书，frp 双向 TLS 校验均信任该 CA。
        </Typography.Paragraph>
        <CertDescriptions info={ca} />
      </Card>
    </Space>
  )
}
