import { useState } from 'react'
import { Button, Input, Select, Space, Typography, message } from 'antd'
import { SearchOutlined, DownOutlined, RightOutlined } from '@ant-design/icons'

const DOH_PROVIDERS = [
  { label: 'Cloudflare', value: 'https://cloudflare-dns.com/dns-query' },
  { label: 'Google', value: 'https://dns.google/dns-query' },
  { label: 'Quad9', value: 'https://dns.quad9.net/dns-query' },
  { label: 'AliDNS', value: 'https://dns.alidns.com/dns-query' },
]

interface DNSResolverProps {
  onResolved: (ip: string) => void
}

export default function DNSResolver({ onResolved }: DNSResolverProps) {
  const [open, setOpen] = useState(false)
  const [domain, setDomain] = useState('')
  const [provider, setProvider] = useState(DOH_PROVIDERS[0].value)
  const [loading, setLoading] = useState(false)

  const resolve = async () => {
    const qname = domain.trim()
    if (!qname) {
      message.warning('请输入域名')
      return
    }
    setLoading(true)
    try {
      const url = `${provider}?name=${encodeURIComponent(qname)}&type=A`
      const res = await fetch(url, { headers: { Accept: 'application/dns-json' } })
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}${res.status === 400 ? '，域名格式可能不正确' : ''}`)
      }
      const data = await res.json()
      if (data.Status !== 0) {
        message.error(`DNS 查询失败 (Status: ${data.Status})`)
        return
      }
      const ips: string[] = (data.Answer || []).filter((a: any) => a.type === 1).map((a: any) => a.data)
      if (ips.length === 0) {
        message.warning('未找到 A 记录')
        return
      }
      onResolved(ips[0])
      message.success(`已解析 ${qname} → ${ips[0]}${ips.length > 1 ? `（还有 ${ips.length - 1} 个备用 IP）` : ''}`)
      setOpen(false)
    } catch (err: any) {
      message.error(`解析失败: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={{ marginTop: 8 }}>
      <Typography.Link onClick={() => setOpen(!open)} style={{ fontSize: 13 }}>
        {open ? <DownOutlined /> : <RightOutlined />} 通过域名解析 IP
      </Typography.Link>
      {open && (
        <Space direction="vertical" size={6} style={{ width: '100%', marginTop: 8 }}>
          <Input
            placeholder="例如 example.com"
            value={domain}
            onChange={(e) => setDomain(e.target.value)}
            onPressEnter={resolve}
            size="small"
          />
          <Space>
            <Select
              value={provider}
              onChange={setProvider}
              options={DOH_PROVIDERS}
              size="small"
              style={{ width: 130 }}
            />
            <Button type="primary" size="small" icon={<SearchOutlined />} loading={loading} onClick={resolve}>
              解析
            </Button>
          </Space>
        </Space>
      )}
    </div>
  )
}
