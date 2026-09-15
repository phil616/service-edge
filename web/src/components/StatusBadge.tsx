import { Badge } from 'antd'

const map: Record<string, { status: 'success' | 'error' | 'default' | 'warning'; text: string }> = {
  online: { status: 'success', text: '在线' },
  offline: { status: 'error', text: '离线' },
  unknown: { status: 'default', text: '状态未知' },
  degraded: { status: 'warning', text: '代理异常' },
  idle: { status: 'default', text: '无活动代理' },
  pending: { status: 'default', text: '待部署' },
}

export default function StatusBadge({ status }: { status: string }) {
  const s = map[status] ?? { status: 'default' as const, text: status }
  return <Badge status={s.status} text={s.text} />
}
