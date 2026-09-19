import { apiClient } from '../client'

export interface StatePoolConfig {
  proxy_id: number
  proxy_name: string
  available: boolean
  issue: string
}
export interface StatePoolRow {
  account_id: number
  account_name: string
  account_status: string
  plan: string
  model: string
  enabled: boolean
  expected_length: number
  state_length: number
  state_status: 'valid' | 'expired' | 'missing' | 'invalid' | 'unknown_plan' | 'disabled'
  issued_at?: string
  mode?: string
  next_probe_at?: string
  obtained_at?: string
  expires_at?: string
  probe_status: 'idle' | 'queued' | 'running' | 'stopped' | 'blocked'
  attempts: number
  last_attempt_at?: string
  reason: string
  can_refresh: boolean
}
export interface StatePoolResponse {
  server_time: string
  config: StatePoolConfig
  models: string[]
  items: StatePoolRow[]
}
export async function getStatePool(signal?: AbortSignal): Promise<StatePoolResponse> {
  const { data } = await apiClient.get<StatePoolResponse>('/admin/accounts/codex-state-pool', { signal })
  return data
}
export async function saveStatePoolProxy(proxyId: number): Promise<StatePoolConfig> {
  const { data } = await apiClient.put<StatePoolConfig>('/admin/accounts/codex-state-pool/config', { proxy_id: proxyId })
  return data
}
export async function refreshState(row: StatePoolRow): Promise<void> {
  await apiClient.post(`/admin/accounts/${row.account_id}/codex-turn-state-probe/refresh`, { model: row.model })
}
export async function getStateValue(row: StatePoolRow): Promise<string> {
  const { data } = await apiClient.get<{ state: string }>(`/admin/accounts/${row.account_id}/codex-turn-state-probe/state`, { params: { model: row.model } })
  return data.state
}

export interface StateEvent {
  id: number; account_id: number; model: string; kind: string; source: string; reason: string; attempt: number
  proxy_id: number; proxy_name: string; proxy_address: string; reference_ip: string; ip_status: string
  http_status: number; sent_length: number; returned_length: number; duration_ms: number; created_at: string
}
export async function getStateEvents(row: StatePoolRow, before = 0): Promise<StateEvent[]> {
  const { data } = await apiClient.get<{items: StateEvent[]}>(`/admin/accounts/${row.account_id}/codex-turn-state-probe/events`, { params: { model: row.model, before } })
  return data.items
}
export interface StateImportJob {
  id: string; account_id: number; model: string; status: 'queued' | 'running' | 'succeeded' | 'failed'; reason?: string; created_at: string; updated_at: string
}
export async function importState(row: StatePoolRow, state: string): Promise<StateImportJob> {
  const { data } = await apiClient.post<StateImportJob>(`/admin/accounts/${row.account_id}/codex-turn-state-probe/import`, { model: row.model, state }, { timeout: 10000 })
  return data
}
export async function getImportJob(row: StatePoolRow, jobId: string): Promise<StateImportJob> {
  const { data } = await apiClient.get<StateImportJob>(`/admin/accounts/${row.account_id}/codex-turn-state-probe/import/${encodeURIComponent(jobId)}`)
  return data
}
