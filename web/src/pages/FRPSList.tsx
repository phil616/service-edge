import { Button, Card, Popconfirm, Space, Table, Typography } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { deleteFRPS, listFRPS } from '../api/client'
import StatusBadge from '../components/StatusBadge'
import type { FRPSNode } from '../api/types'

export default function FRPSList() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const { data, isLoading } = useQuery({ queryKey: ['frps'], queryFn: listFRPS })
  const del = useMutation({
    mutationFn: deleteFRPS,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['frps'] })
      qc.invalidateQueries({ queryKey: ['topology'] })
    },
  })

  const columns = [
    { title: '名称', dataIndex: 'name', render: (v: string, r: FRPSNode) => <a onClick={() => navigate(`/frps/${r.uuid}`)}>{v}</a> },
    { title: 'UUID', dataIndex: 'uuid', render: (v: string) => <span className="mono">{v.slice(0, 8)}</span> },
    { title: '服务端口', dataIndex: 'bind_port' },
    { title: '公网 IP', dataIndex: 'public_ip', render: (v: string) => v || '-' },
    { title: '状态', dataIndex: 'status', render: (v: string) => <StatusBadge status={v} /> },
    { title: '配置版本', dataIndex: 'config_version' },
    {
      title: '最后心跳',
      dataIndex: 'last_heartbeat',
      render: (v: string | null) => (v ? dayjs(v).format('MM-DD HH:mm:ss') : '-'),
    },
    {
      title: '操作',
      render: (_: unknown, r: FRPSNode) => (
        <Space>
          <a onClick={() => navigate(`/frps/${r.uuid}`)}>详情</a>
          <Popconfirm title="确认删除该节点？" onConfirm={() => del.mutate(r.uuid)}>
            <a style={{ color: '#cf1322' }}>删除</a>
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <Card
      title={<Typography.Title level={4} style={{ margin: 0 }}>FRPS 节点</Typography.Title>}
      extra={
        <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/frps/new')}>
          创建节点
        </Button>
      }
    >
      <Table rowKey="uuid" loading={isLoading} dataSource={data} columns={columns} />
    </Card>
  )
}
