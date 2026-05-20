import { Card, Descriptions, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import { fetchMe } from '../api/client'

export default function Settings() {
  const { data } = useQuery({ queryKey: ['me'], queryFn: fetchMe })

  return (
    <Card title={<Typography.Title level={4} style={{ margin: 0 }}>系统设置</Typography.Title>}>
      <Descriptions column={1} bordered size="small">
        <Descriptions.Item label="当前用户">{data?.username}</Descriptions.Item>
        <Descriptions.Item label="用户 ID">{data?.id}</Descriptions.Item>
      </Descriptions>
      <Typography.Paragraph type="secondary" style={{ marginTop: 16 }}>
        前后端分离部署：前端 edge.dreamreflex.com，后端 edge-api.dreamreflex.com。认证使用 Bearer JWT，CORS 白名单限定前端域名。
      </Typography.Paragraph>
    </Card>
  )
}
