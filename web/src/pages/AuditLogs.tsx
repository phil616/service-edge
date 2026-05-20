import { useState } from 'react'
import { Card, Table, Tag, Typography } from 'antd'
import { useQuery } from '@tanstack/react-query'
import dayjs from 'dayjs'
import { listAuditLogs } from '../api/client'
import type { AuditLog } from '../api/types'

export default function AuditLogs() {
  const [page, setPage] = useState(1)
  const pageSize = 20
  const { data, isLoading } = useQuery({
    queryKey: ['audit', page],
    queryFn: () => listAuditLogs(pageSize, (page - 1) * pageSize),
  })

  const columns = [
    { title: '时间', dataIndex: 'created_at', render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss') },
    { title: '操作', dataIndex: 'action', render: (v: string) => <Tag>{v}</Tag> },
    { title: '对象类型', dataIndex: 'target_type', render: (v: string) => v || '-' },
    { title: '对象', dataIndex: 'target_uuid', render: (v: string) => <span className="mono">{v ? v.slice(0, 12) : '-'}</span> },
    { title: '详情', dataIndex: 'detail', render: (v: string) => v || '-' },
    { title: 'IP', dataIndex: 'ip', render: (v: string) => v || '-' },
  ]

  return (
    <Card title={<Typography.Title level={4} style={{ margin: 0 }}>审计日志</Typography.Title>}>
      <Table<AuditLog>
        rowKey="id"
        loading={isLoading}
        dataSource={data?.items ?? []}
        columns={columns}
        pagination={{
          current: page,
          pageSize,
          total: data?.total ?? 0,
          onChange: setPage,
          showSizeChanger: false,
        }}
      />
    </Card>
  )
}
