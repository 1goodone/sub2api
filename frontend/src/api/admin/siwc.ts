import { apiClient } from '../client'

export interface SIWCStatus {
  bound: boolean
  refreshable?: boolean
  enabled: boolean
  expires_at?: number
  task?: { status: string; error?: string; stage: string }
}
export interface SIWCListStatus {
  account_id: number
  bound: boolean
  refreshable: boolean
  enabled: boolean
  task?: { status: string; error?: string; stage: string }
}
export interface SIWCAuthorization { auth_url: string; session_id: string }
const path = (id: number) => `/admin/accounts/${id}/siwc`
export async function getSIWCStatus(id: number): Promise<SIWCStatus> {
  return (await apiClient.get(path(id))).data
}
export async function authorizeSIWC(id: number): Promise<SIWCAuthorization> {
  return (await apiClient.post(`${path(id)}/authorize`)).data
}
export async function completeSIWC(id: number, sessionId: string, callbackUrl: string): Promise<void> {
  await apiClient.post(`${path(id)}/callback`, { session_id: sessionId, callback_url: callbackUrl })
}
export async function loginSIWCWithSavedCredentials(id: number): Promise<void> {
  await apiClient.post(`/admin/accounts/${id}/openai-reauth`, { oauth_profile: 'siwc' })
}
export async function listSIWCStatuses(ids: number[]): Promise<SIWCListStatus[]> {
  const data = (await apiClient.post<{ accounts?: SIWCListStatus[] }>('/admin/accounts/siwc-status', { ids })).data
  return data.accounts ?? []
}
export async function setSIWCEnabled(id: number, enabled: boolean): Promise<void> {
  await apiClient.put(`${path(id)}/enabled`, { enabled })
}
