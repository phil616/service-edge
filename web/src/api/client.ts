import axios from 'axios'
import { message } from 'antd'
import { useAuth } from '../store/auth'
import type {
  AgentDownloadSettings,
  AuditLog,
  CertInfo,
  FRPCConnection,
  FRPCHost,
  FRPDistFile,
  FRPSNode,
  InstallCommandResponse,
  PortAvailability,
  PortUse,
  ProxyInput,
  ProxyMapping,
  Topology,
  User,
} from './types'

export const http = axios.create({ baseURL: '/' })

http.interceptors.request.use((config) => {
  const token = useAuth.getState().token
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

http.interceptors.response.use(
  (resp) => resp,
  (error) => {
    const status = error.response?.status
    if (status === 401) {
      // Token expired/invalid: drop it and bounce to login.
      useAuth.getState().clear()
      if (location.pathname !== '/login') location.assign('/login')
    } else {
      const msg = error.response?.data?.error || error.message || 'request failed'
      message.error(msg)
    }
    return Promise.reject(error)
  },
)

// ---- auth ----
export async function login(username: string, password: string) {
  const { data } = await http.post<{ token: string; user: User }>('/api/v1/auth/login', { username, password })
  return data
}
export async function fetchMe() {
  const { data } = await http.get<User>('/api/v1/auth/me')
  return data
}
export async function logout() {
  await http.post('/api/v1/auth/logout')
}

// ---- frps ----
export async function listFRPS() {
  const { data } = await http.get<{ items: FRPSNode[] }>('/api/v1/frps')
  return data.items ?? []
}
export async function getFRPS(uuid: string) {
  const { data } = await http.get<FRPSNode>(`/api/v1/frps/${uuid}`)
  return data
}
export async function createFRPS(body: Record<string, unknown>) {
  const { data } = await http.post<FRPSNode>('/api/v1/frps', body)
  return data
}
export async function updateFRPS(uuid: string, body: Record<string, unknown>) {
  const { data } = await http.put<FRPSNode>(`/api/v1/frps/${uuid}`, body)
  return data
}
export async function deleteFRPS(uuid: string) {
  await http.delete(`/api/v1/frps/${uuid}`)
}
export async function frpsInstallCommand(uuid: string) {
  const { data } = await http.post<InstallCommandResponse>(`/api/v1/frps/${uuid}/install-command`)
  return data
}
export async function frpsUsedPorts(uuid: string) {
  const { data } = await http.get<PortAvailability>(`/api/v1/frps/${uuid}/available-ports`)
  return data.used_ports ?? []
}
export async function frpsPortAvailability(uuid: string) {
  const { data } = await http.get<PortAvailability>(`/api/v1/frps/${uuid}/available-ports`)
  return { used_ports: data.used_ports ?? [], host_occupied_ports: data.host_occupied_ports ?? [] }
}
export async function frpsPortUsage(uuid: string) {
  const { data } = await http.get<{ items: PortUse[] }>(`/api/v1/frps/${uuid}/port-usage`)
  return data.items ?? []
}

// ---- frpc hosts ----
export async function listFRPCHosts() {
  const { data } = await http.get<{ items: FRPCHost[] }>('/api/v1/frpc-hosts')
  return data.items ?? []
}
export async function getFRPCHost(uuid: string) {
  const { data } = await http.get<FRPCHost>(`/api/v1/frpc-hosts/${uuid}`)
  return data
}
export async function createFRPCHost(body: Record<string, unknown>) {
  const { data } = await http.post<FRPCHost>('/api/v1/frpc-hosts', body)
  return data
}
export async function updateFRPCHost(uuid: string, body: Record<string, unknown>) {
  const { data } = await http.put<FRPCHost>(`/api/v1/frpc-hosts/${uuid}`, body)
  return data
}
export async function deleteFRPCHost(uuid: string) {
  await http.delete(`/api/v1/frpc-hosts/${uuid}`)
}
export async function frpcHostInstallCommand(uuid: string) {
  const { data } = await http.post<InstallCommandResponse>(`/api/v1/frpc-hosts/${uuid}/install-command`)
  return data
}

// ---- frpc connections ----
export async function listConnections(hostUUID: string) {
  const { data } = await http.get<{ items: FRPCConnection[] }>(`/api/v1/frpc-hosts/${hostUUID}/connections`)
  return data.items ?? []
}
export async function createConnection(hostUUID: string, body: Record<string, unknown>) {
  const { data } = await http.post<FRPCConnection>(`/api/v1/frpc-hosts/${hostUUID}/connections`, body)
  return data
}
export async function getConnection(uuid: string) {
  const { data } = await http.get<FRPCConnection>(`/api/v1/connections/${uuid}`)
  return data
}
export async function updateConnection(uuid: string, body: Record<string, unknown>) {
  const { data } = await http.put<FRPCConnection>(`/api/v1/connections/${uuid}`, body)
  return data
}
export async function deleteConnection(uuid: string) {
  await http.delete(`/api/v1/connections/${uuid}`)
}

// ---- proxies (belong to a connection) ----
export async function listProxies(connUUID: string) {
  const { data } = await http.get<{ items: ProxyMapping[] }>(`/api/v1/connections/${connUUID}/proxies`)
  return data.items ?? []
}
export async function addProxy(connUUID: string, body: ProxyInput) {
  const { data } = await http.post<ProxyMapping>(`/api/v1/connections/${connUUID}/proxies`, body)
  return data
}
export async function updateProxy(id: number, body: ProxyInput) {
  const { data } = await http.put<ProxyMapping>(`/api/v1/proxies/${id}`, body)
  return data
}
export async function deleteProxy(id: number) {
  await http.delete(`/api/v1/proxies/${id}`)
}

// ---- insights ----
export async function getCAInfo() {
  const { data } = await http.get<CertInfo>('/api/v1/ca')
  return data
}
export async function getTopology() {
  const { data } = await http.get<Topology>('/api/v1/topology')
  return data
}
export async function getSettings() {
  const { data } = await http.get<AgentDownloadSettings>('/api/v1/settings')
  return data
}
export async function updateSettings(body: { agent_download_url_frps: string; agent_download_url_frpc: string }) {
  const { data } = await http.put<AgentDownloadSettings>('/api/v1/settings', body)
  return data
}

// ---- frp dist ----
export async function listFRPDists() {
  const { data } = await http.get<{ items: FRPDistFile[] }>('/api/v1/frp-dist')
  return data.items ?? []
}
export async function uploadFRPDist(file: File) {
  const form = new FormData()
  form.append('file', file)
  const { data } = await http.post<{ items: FRPDistFile[] }>('/api/v1/frp-dist', form, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return data.items ?? []
}
export async function deleteFRPDist(id: number) {
  await http.delete(`/api/v1/frp-dist/${id}`)
}

// ---- audit ----
export async function listAuditLogs(limit = 100, offset = 0) {
  const { data } = await http.get<{ items: AuditLog[]; total: number }>(
    `/api/v1/audit-logs?limit=${limit}&offset=${offset}`,
  )
  return data
}
