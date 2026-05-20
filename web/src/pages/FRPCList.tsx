import { Button, Card, Popconfirm, Space, Table, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { deleteFRPC, listFRPC, listFRPS } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import type { FRPCClient } from '../api/types'

export default function FRPCList() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['frpc'], queryFn: listFRPC })
  const { data: nodes } = useQuery({ queryKey: ['frps'], queryFn: listFRPS })
  const del = useMutation({
    mutationFn: deleteFRPC,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['frpc'] })
      qc.invalidateQueries({ queryKey: ['topology'] })
    },
  })

  const nodeName = (uuid: string) => nodes?.find((n) => n.uuid === uuid)?.name ?? uuid.slice(0, 8)

  const columns = [
    { title: '名称', dataIndex: 'name', render: (v: string, r: FRPCClient) => <a onClick={() => navigate(`/frpc/${r.uuid}`)}>{v}</a> },
    { title: 'UUID', dataIndex: 'uuid', render: (v: string) => <span className="mono">{v.slice(0, 8)}</span> },
    { title: '目标 FRPS', dataIndex: 'frps_uuid', render: (v: string) => nodeName(v) },
    { title: '状态', dataIndex: 'status', render: (v: string) => <StatusBadge status={v} /> },
    { title: '配置版本', dataIndex: 'config_version' },
    {
      title: '最后心跳',
      dataIndex: 'last_heartbeat',
      render: (v: string | null) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-'),
    },
    {
      title: '操作',
      render: (_: unknown, r: FRPCClient) => (
        <Space>
          <a onClick={() => navigate(`/frpc/${r.uuid}`)}>详情</a>
          <Popconfirm title="确认删除该客户端？" onConfirm={() => del.mutate(r.uuid)}>
            <a style={{ color: '#cf1322' }}>删除</a>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={<Typography.Title level={4} style={{ margin: 0 }}>FRPC 客户端</Typography.Title>}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/frpc/new')}>
          创建客户端
        </Button>
      }
    >
      <Table rowKey="uuid" loading={isLoading} dataSource={data} columns={columns} />
    </Card>
  )
}
