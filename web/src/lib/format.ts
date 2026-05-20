// Human-readable formatting helpers shared across detail/dashboard views.

export function formatUptime(sec?: number): string {
  if (!sec || sec <= 0) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const parts: string[] = []
  if (d) parts.push(`${d}天`)
  if (h) parts.push(`${h}小时`)
  if (m || parts.length === 0) parts.push(`${m}分钟`)
  return parts.join(' ')
}

export function formatMemoryMB(mb?: number): string {
  if (!mb || mb <= 0) return '-'
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GiB`
  return `${mb} MiB`
}

export function archLabel(os?: string, arch?: string): string {
  if (!os && !arch) return '-'
  return [os, arch].filter(Boolean).join(' / ')
}
