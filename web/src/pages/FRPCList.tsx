import { Button, Card, Popconfirm, Space, Table, Tag, Tooltip, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { deleteFRPCHost, listFRPCHosts } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import type { FRPCHost } from '../api/types'

export default function FRPCList() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['frpc-hosts'], queryFn: listFRPCHosts, refetchInterval: 10000 })
  const del = useMutation({
    mutationFn: deleteFRPCHost,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['frpc-hosts'] })
      qc.invalidateQueries({ queryKey: ['topology'] })
    },
  })

  const columns = [
    { title: '主机名称', dataIndex: 'name', render: (v: string, r: FRPCHost) => <a onClick={() => navigate(`/frpc/${r.uuid}`)}>{v}</a> },
    { title: 'UUID', dataIndex: 'uuid', render: (v: string) => <span className="mono">{v.slice(0, 8)}</span> },
    { title: '连接数', render: (_: unknown, r: FRPCHost) => <Tag color="blue">{r.connections?.length ?? 0}</Tag> },
    { title: '状态', dataIndex: 'status', render: (v: string) => <StatusBadge status={v} /> },
    { title: 'frp 版本', dataIndex: 'frp_version', render: (v: string) => <Tag>{v || '-'}</Tag> },
    {
      title: '最后心跳',
      dataIndex: 'last_heartbeat',
      render: (v: string | null) =>
        v ? <Tooltip title={dayjs(v).format('YYYY-MM-DD HH:mm:ss')}>{dayjs(v).fromNow()}</Tooltip> : '-',
    },
    {
      title: '操作',
      render: (_: unknown, r: FRPCHost) => (
        <Space>
          <a onClick={() => navigate(`/frpc/${r.uuid}`)}>详情</a>
          <Popconfirm title="确认删除该主机？其下所有连接将一并删除。" onConfirm={() => del.mutate(r.uuid)}>
            <a style={{ color: '#cf1322' }}>删除</a>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={<Typography.Title level={4} style={{ margin: 0 }}>FRPC 主机</Typography.Title>}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/frpc/new')}>
          创建主机
        </Button>
      }
    >
      <Typography.Paragraph type="secondary" style={{ marginTop: 0 }}>
        一台主机安装一次 Agent，即可在其下创建多个连接，分别连往不同的 FRPS。
      </Typography.Paragraph>
      <Table rowKey="uuid" loading={isLoading} dataSource={data} columns={columns} />
    </Card>
  )
}
