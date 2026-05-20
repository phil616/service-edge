import axios from 'axios'
import { message } from 'antd'
import { useAuth } from '../store/auth'
import type {
  AuditLog,
  FRPCClient,
  FRPSNode,
  InstallCommandResponse,
  ProxyInput,
  ProxyMapping,
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
  const { data } = await http.get<{ used_ports: number[] }>(`/api/v1/frps/${uuid}/available-ports`)
  return data.used_ports ?? []
}

// ---- frpc ----
export async function listFRPC() {
  const { data } = await http.get<{ items: FRPCClient[] }>('/api/v1/frpc')
  return data.items ?? []
}
export async function getFRPC(uuid: string) {
  const { data } = await http.get<FRPCClient>(`/api/v1/frpc/${uuid}`)
  return data
}
export async function createFRPC(body: Record<string, unknown>) {
  const { data } = await http.post<FRPCClient>('/api/v1/frpc', body)
  return data
}
export async function updateFRPC(uuid: string, body: Record<string, unknown>) {
  const { data } = await http.put<FRPCClient>(`/api/v1/frpc/${uuid}`, body)
  return data
}
export async function deleteFRPC(uuid: string) {
  await http.delete(`/api/v1/frpc/${uuid}`)
}
export async function frpcInstallCommand(uuid: string) {
  const { data } = await http.post<InstallCommandResponse>(`/api/v1/frpc/${uuid}/install-command`)
  return data
}

// ---- proxies ----
export async function listProxies(frpcUUID: string) {
  const { data } = await http.get<{ items: ProxyMapping[] }>(`/api/v1/frpc/${frpcUUID}/proxies`)
  return data.items ?? []
}
export async function addProxy(frpcUUID: string, body: ProxyInput) {
  const { data } = await http.post<ProxyMapping>(`/api/v1/frpc/${frpcUUID}/proxies`, body)
  return data
}
export async function updateProxy(id: number, body: ProxyInput) {
  const { data } = await http.put<ProxyMapping>(`/api/v1/proxies/${id}`, body)
  return data
}
export async function deleteProxy(id: number) {
  await http.delete(`/api/v1/proxies/${id}`)
}

// ---- audit ----
export async function listAuditLogs(limit = 100, offset = 0) {
  const { data } = await http.get<{ items: AuditLog[]; total: number }>(
    `/api/v1/audit-logs?limit=${limit}&offset=${offset}`,
  )
  return data
}
