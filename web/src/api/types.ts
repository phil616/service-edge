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

export interface FRPSNode {
  id: number
  uuid: string
  name: string
  bind_port: number
  dashboard_port?: number | null
  dashboard_user?: string
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
  created_at: string
}

export interface FRPCClient {
  id: number
  uuid: string
  name: string
  frps_uuid: string
  frp_version: string
  config_version: number
  status: 'pending' | 'online' | 'offline'
  last_heartbeat?: string | null
  runtime?: AgentRuntime
  tls_cert_info?: CertInfo | null
  created_at: string
  updated_at: string
  proxies?: ProxyMapping[]
}

export interface PortUse {
  port: number
  kind: 'bind' | 'dashboard' | 'proxy'
  frpc_uuid?: string
  frpc_name?: string
  proxy_name?: string
  proxy_type?: string
}

export interface Topology {
  frps: FRPSNode[]
  frpc: FRPCClient[]
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
