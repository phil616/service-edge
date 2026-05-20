export interface User {
  id: number
  username: string
  created_at?: string
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
  created_at: string
  updated_at: string
  proxies?: ProxyMapping[]
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
