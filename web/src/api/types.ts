export interface User {
  id: number
  username: string
  created_at?: string
}

export interface AgentRuntime {
  os?: string
  arch?: string
  kernel?: string
  memory_mb?: number
  uptime_sec?: number
  process_pid?: number
  active_connections?: number
  frp_last_error?: string
  reported_at?: string | null
}

export interface CertInfo {
  subject: string
  issuer: string
  serial_number: string
  not_before: string
  not_after: string
  is_ca: boolean
  dns_names?: string[]
  signature_algorithm: string
  public_key_algorithm: string
  key_bits?: number
  fingerprint_sha256: string
  days_remaining: number
  expired: boolean
}

export type TransportProtocol = 'tcp' | 'kcp' | 'quic' | 'websocket' | 'wss'

export interface FRPSNode {
  id: number
  uuid: string
  name: string
  bind_port: number
  dashboard_port?: number | null
  dashboard_user?: string
  kcp_bind_port?: number | null
  quic_bind_port?: number | null
  frp_version: string
  config_version: number
  status: 'pending' | 'online' | 'offline'
  last_heartbeat?: string | null
  public_ip?: string
  runtime?: AgentRuntime
  tls_cert_info?: CertInfo | null
  created_at: string
  updated_at: string
}

export interface ProxyMapping {
  id: number
  frpc_uuid: string
  name: string
  proxy_type: 'tcp' | 'udp' | 'http' | 'https'
  local_ip: string
  local_port: number
  remote_port?: number | null
  custom_domains?: string
  subdomain?: string
  inactive?: boolean
  inactive_reason?: string
  created_at: string
}

// FRPCHost is a machine running the frpc agent (installed once); it owns many
// connections, one frpc process each.
export interface FRPCHost {
  id: number
  uuid: string
  name: string
  frp_version: string
  config_version: number
  status: 'pending' | 'online' | 'offline'
  last_heartbeat?: string | null
  runtime?: AgentRuntime
  created_at: string
  updated_at: string
  connections?: FRPCConnection[]
}

// FRPCConnection is one frpc process: host -> one frps, with its own transport,
// admin port and proxies.
export interface FRPCConnection {
  id: number
  uuid: string
  host_uuid: string
  name: string
  frps_uuid: string
  protocol?: TransportProtocol
  admin_port: number
  config_version: number
  status: 'pending' | 'online' | 'offline'
  last_heartbeat?: string | null
  tls_cert_info?: CertInfo | null
  created_at: string
  updated_at: string
  proxies?: ProxyMapping[]
}

export interface PortUse {
  port: number
  kind: 'bind' | 'dashboard' | 'proxy' | 'host' | 'kcp' | 'quic'
  frpc_uuid?: string
  frpc_name?: string
  proxy_name?: string
  proxy_type?: string
}

export interface PortAvailability {
  used_ports: number[]
  host_occupied_ports: number[]
}

export interface Topology {
  frps: FRPSNode[]
  hosts: FRPCHost[]
}

export interface AgentDownloadSettings {
  control_plane_base: string
  agent_download_url_frps: string
  agent_download_url_frpc: string
}

export interface AuditLog {
  id: number
  user_id?: number
  action: string
  target_type?: string
  target_uuid?: string
  detail?: string
  ip?: string
  created_at: string
}

export interface InstallCommandResponse {
  command: string
  token: string
  expires_at: string
}

export interface ProxyInput {
  name: string
  proxy_type: string
  local_ip?: string
  local_port: number
  remote_port?: number | null
  custom_domains?: string[]
  subdomain?: string
}
