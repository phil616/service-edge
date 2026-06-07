// frp-related shared helpers.

import type { FRPDistFile } from '../api/types'

function versionParts(v: string): number[] {
  return v
    .replace(/^v/, '')
    .trim()
    .split('.')
    .map((f) => parseInt(f, 10) || 0)
}

function compareVersion(a: string, b: string): number {
  const pa = versionParts(a)
  const pb = versionParts(b)
  const n = Math.max(pa.length, pb.length)
  for (let i = 0; i < n; i++) {
    const x = pa[i] ?? 0
    const y = pb[i] ?? 0
    if (x !== y) return x - y
  }
  return 0
}

// latestFrpVersion returns the highest (by semantic version) version among the
// uploaded frp dist files, or '' when none have been uploaded. Used to default
// the "FRP 版本" field to a release that actually exists in dist management.
export function latestFrpVersion(dists: Pick<FRPDistFile, 'version'>[]): string {
  let best = ''
  for (const d of dists) {
    if (!d.version) continue
    if (!best || compareVersion(d.version, best) > 0) best = d.version
  }
  return best
}

// maskIp masks an IP/address for screen-sharing/recording: when hide is on, any
// non-empty value becomes "*.*.*.*". Empty values pass through unchanged so
// placeholders like "未设置" are preserved by the caller.
export function maskIp(value: string | undefined | null, hide: boolean): string {
  const v = value ?? ''
  if (hide && v) return '*.*.*.*'
  return v
}
