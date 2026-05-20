import { useEffect, useState } from 'react'
import { Descriptions, Popover, Tag, Typography } from 'antd'
import { QuestionCircleOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'

// Default agent cadences (agent.yaml defaults; control plane long-poll = 30s).
// These are the standard values; a host with a customized agent.yaml may differ.
const HEARTBEAT_SEC = 20
const STATUS_SEC = 180
const CONFIG_POLL_SEC = 30
// Liveness reaper marks a node offline after this long without a heartbeat.
const LIVENESS_TIMEOUT_SEC = 60

const abs = (ts?: string | null) => (ts ? dayjs(ts).format('YYYY-MM-DD HH:mm:ss') : '—')
const rel = (ts?: string | null) => (ts ? dayjs(ts).fromNow() : '—')

// useTick forces a re-render on an interval so relative times stay current.
function useTick(ms = 1000) {
  const [, setN] = useState(0)
  useEffect(() => {
    const id = setInterval(() => setN((n) => n + 1), ms)
    return () => clearInterval(id)
  }, [ms])
}

const mechanism = (
  <div style={{ maxWidth: 360, fontSize: 13, lineHeight: 1.8 }}>
    <p style={{ margin: '0 0 8px' }}>Agent 与控制面通过三条独立的循环保持同步：</p>
    <ul style={{ paddingLeft: 18, margin: 0 }}>
      <li>
        <Typography.Text strong>心跳</Typography.Text>：每 ~{HEARTBEAT_SEC}s 上报存活与进程状态；超过 {LIVENESS_TIMEOUT_SEC}s 未收到则判定离线。
      </li>
      <li>
        <Typography.Text strong>状态上报</Typography.Text>：每 ~{STATUS_SEC}s 上报主机详情、监听端口与各映射的 frp 实时状态（配置应用后也会立即补报一次）。
      </li>
      <li>
        <Typography.Text strong>配置下发</Typography.Text>：采用长轮询，连接持续挂起；控制面一旦有变更会<Typography.Text strong>立即唤醒</Typography.Text>下发，空闲时每 ~{CONFIG_POLL_SEC}s 复查一次。
      </li>
    </ul>
    <p style={{ margin: '8px 0 0', color: '#888' }}>因此配置变更通常数秒内生效，最长约 {CONFIG_POLL_SEC}s。</p>
  </div>
)

export default function AgentSyncInfo({
  lastHeartbeat,
  reportedAt,
  status,
}: {
  lastHeartbeat?: string | null
  reportedAt?: string | null
  status?: string
}) {
  useTick(1000)

  const online = status === 'online'
  const nextStatus = reportedAt ? dayjs(reportedAt).add(STATUS_SEC, 'second') : null
  // The long-poll completes (and re-polls) within CONFIG_POLL_SEC of the last
  // contact; use the freshest heartbeat as the contact marker.
  const nextConfigCheck = lastHeartbeat ? dayjs(lastHeartbeat).add(CONFIG_POLL_SEC, 'second') : null

  const hbColor = !lastHeartbeat ? 'default' : online ? 'success' : 'error'

  return (
    <Descriptions
      column={1}
      size="small"
      bordered
      title={
        <span>
          同步状态{' '}
          <Popover content={mechanism} title="刷新机制">
            <QuestionCircleOutlined style={{ color: '#999', cursor: 'help' }} />
          </Popover>
        </span>
      }
    >
      <Descriptions.Item label="上次心跳">
        {lastHeartbeat ? (
          <>
            <Tag color={hbColor}>{rel(lastHeartbeat)}</Tag>
            <span className="mono" style={{ color: '#888' }}>{abs(lastHeartbeat)}</span>
          </>
        ) : (
          <Typography.Text type="secondary">尚未上报</Typography.Text>
        )}
      </Descriptions.Item>

      <Descriptions.Item label="上次详细上报">
        {reportedAt ? (
          <>
            <Tag>{rel(reportedAt)}</Tag>
            <span className="mono" style={{ color: '#888' }}>{abs(reportedAt)}</span>
          </>
        ) : (
          <Typography.Text type="secondary">尚未上报</Typography.Text>
        )}
      </Descriptions.Item>

      <Descriptions.Item label="预计下次详细上报">
        {nextStatus ? (
          <>
            <Tag color="blue">{nextStatus.fromNow()}</Tag>
            <span className="mono" style={{ color: '#888' }}>{nextStatus.format('HH:mm:ss')}</span>
          </>
        ) : (
          '—'
        )}
      </Descriptions.Item>

      <Descriptions.Item label="预计下次配置检查">
        {nextConfigCheck ? (
          <>
            <Tag color="geekblue">{nextConfigCheck.fromNow()}</Tag>
            <Typography.Text type="secondary">最长约 {CONFIG_POLL_SEC}s；配置变更会立即下发</Typography.Text>
          </>
        ) : (
          <Typography.Text type="secondary">长轮询实时下发（≤ {CONFIG_POLL_SEC}s）</Typography.Text>
        )}
      </Descriptions.Item>
    </Descriptions>
  )
}
