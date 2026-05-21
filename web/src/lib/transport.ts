import type { FRPSNode, TransportProtocol } from '../api/types'

// Display labels for frpc<->frps control transports.
export const PROTOCOL_LABELS: Record<TransportProtocol, string> = {
  tcp: 'TCP',
  kcp: 'KCP',
  quic: 'QUIC',
  websocket: 'WebSocket',
  wss: 'WSS',
}

// protocolLabel maps an arbitrary protocol string to its display label (TCP fallback).
export function protocolLabel(p?: string): string {
  return PROTOCOL_LABELS[p as TransportProtocol] ?? (p || 'TCP')
}

// nodeProtocols lists the transports a node offers: TCP / WebSocket / WSS always
// (they ride the TCP bind port); KCP / QUIC only when the node enabled their port.
export function nodeProtocols(node?: Pick<FRPSNode, 'kcp_bind_port' | 'quic_bind_port'> | null): TransportProtocol[] {
  const out: TransportProtocol[] = ['tcp']
  if (node?.kcp_bind_port) out.push('kcp')
  if (node?.quic_bind_port) out.push('quic')
  out.push('websocket', 'wss')
  return out
}
