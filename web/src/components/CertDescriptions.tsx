import { Descriptions, Empty, Tag } from 'antd'
import dayjs from 'dayjs'
import type { CertInfo } from '../api/types'

function expiryTag(info: CertInfo) {
  if (info.expired) return <Tag color="red">已过期</Tag>
  if (info.days_remaining <= 30) return <Tag color="orange">{info.days_remaining} 天后过期</Tag>
  return <Tag color="green">剩余 {info.days_remaining} 天</Tag>
}

export default function CertDescriptions({ info }: { info?: CertInfo | null }) {
  if (!info) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="无证书信息" />
  }
  return (
    <Descriptions column={1} bordered size="small">
      <Descriptions.Item label="主题 (Subject)"><span className="mono">{info.subject}</span></Descriptions.Item>
      <Descriptions.Item label="签发者 (Issuer)"><span className="mono">{info.issuer}</span></Descriptions.Item>
      <Descriptions.Item label="类型">{info.is_ca ? <Tag color="purple">CA 根证书</Tag> : <Tag>叶子证书</Tag>}</Descriptions.Item>
      {info.dns_names && info.dns_names.length > 0 && (
        <Descriptions.Item label="SAN">
          {info.dns_names.map((d) => <Tag key={d}>{d}</Tag>)}
        </Descriptions.Item>
      )}
      <Descriptions.Item label="公钥算法">{info.public_key_algorithm}{info.key_bits ? ` (${info.key_bits} bit)` : ''}</Descriptions.Item>
      <Descriptions.Item label="签名算法">{info.signature_algorithm}</Descriptions.Item>
      <Descriptions.Item label="序列号"><span className="mono" style={{ wordBreak: 'break-all' }}>{info.serial_number}</span></Descriptions.Item>
      <Descriptions.Item label="生效时间">{dayjs(info.not_before).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
      <Descriptions.Item label="过期时间">
        {dayjs(info.not_after).format('YYYY-MM-DD HH:mm:ss')} {expiryTag(info)}
      </Descriptions.Item>
      <Descriptions.Item label="SHA-256 指纹"><span className="mono" style={{ wordBreak: 'break-all' }}>{info.fingerprint_sha256}</span></Descriptions.Item>
    </Descriptions>
  )
}
