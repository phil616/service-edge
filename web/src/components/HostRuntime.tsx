import { Descriptions, Empty, Tag } from 'antd'
import dayjs from 'dayjs'
import type { AgentRuntime } from '../api/types'
import { archLabel, formatMemoryMB, formatUptime } from '../lib/format'

export default function HostRuntime({ runtime }: { runtime?: AgentRuntime }) {
  if (!runtime || !runtime.reported_at) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="尚未收到 Agent 状态上报" />
  }
  return (
    <Descriptions column={1} bordered size="small">
      <Descriptions.Item label="操作系统 / 架构">
        <Tag color="geekblue">{archLabel(runtime.os, runtime.arch)}</Tag>
      </Descriptions.Item>
      <Descriptions.Item label="内核版本"><span className="mono">{runtime.kernel || '-'}</span></Descriptions.Item>
      <Descriptions.Item label="总内存">{formatMemoryMB(runtime.memory_mb)}</Descriptions.Item>
      <Descriptions.Item label="运行时长">{formatUptime(runtime.uptime_sec)}</Descriptions.Item>
      <Descriptions.Item label="frp 进程 PID">{runtime.process_pid || '-'}</Descriptions.Item>
      <Descriptions.Item label="活动连接数">{runtime.active_connections ?? 0}</Descriptions.Item>
      <Descriptions.Item label="frp 最近错误">
        {runtime.frp_last_error ? <span style={{ color: '#cf1322' }}>{runtime.frp_last_error}</span> : '无'}
      </Descriptions.Item>
      <Descriptions.Item label="上报时间">
        {runtime.reported_at ? dayjs(runtime.reported_at).format('YYYY-MM-DD HH:mm:ss') : '-'}
      </Descriptions.Item>
    </Descriptions>
  )
}
