import { useEffect, useState } from 'react'
import { Alert, Button, Input, Space, Typography, message } from 'antd'
import { CopyOutlined, ReloadOutlined } from '@ant-design/icons'
import type { InstallCommandResponse } from '../api/types'

interface Props {
  generate: () => Promise<InstallCommandResponse>
}

function useCountdown(expiresAt?: string) {
  const [remaining, setRemaining] = useState(0)
  useEffect(() => {
    if (!expiresAt) return
    const target = new Date(expiresAt).getTime()
    const tick = () => setRemaining(Math.max(0, Math.floor((target - Date.now()) / 1000)))
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [expiresAt])
  return remaining
}

export default function InstallCommand({ generate }: Props) {
  const [cmd, setCmd] = useState<InstallCommandResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const remaining = useCountdown(cmd?.expires_at)
  const expired = cmd != null && remaining <= 0

  const run = async () => {
    setLoading(true)
    try {
      setCmd(await generate())
    } finally {
      setLoading(false)
    }
  }

  const copy = async () => {
    if (!cmd) return
    try {
      await navigator.clipboard.writeText(cmd.command)
      message.success('已复制到剪贴板')
    } catch {
      message.warning('无法访问剪贴板，请手动复制')
    }
  }

  const mmss = `${String(Math.floor(remaining / 60)).padStart(2, '0')}:${String(remaining % 60).padStart(2, '0')}`

  return (
    <Space direction="vertical" style={{ width: '100%' }}>
      {!cmd && (
        <Button type="primary" loading={loading} onClick={run}>
          生成一次性安装命令
        </Button>
      )}
      {cmd && (
        <>
          <Alert
            type={expired ? 'warning' : 'info'}
            showIcon
            message={
              expired
                ? '安装命令已过期，请重新生成'
                : `安装命令有效，剩余 ${mmss}（一次性使用，部署后自动失效）`
            }
          />
          <Input.TextArea className="mono" value={cmd.command} autoSize readOnly />
          <Space>
            <Button icon={<CopyOutlined />} onClick={copy} disabled={expired}>
              复制命令
            </Button>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={run}>
              重新生成
            </Button>
          </Space>
          <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
            在目标机器以 root 执行此命令即可完成安装与注册。
          </Typography.Paragraph>
        </>
      )}
    </Space>
  )
}
