import { apiClient } from './client'
import type {
  ApiKey,
  ApiKeyPackage,
  ApiKeyPackagePlan,
  BindPackageApiKeyRequest,
  PaginatedResponse,
  SaveApiKeyPackageRequest,
  UpgradePackageApiKeyRequest,
} from '@/types'

const idempotencyKey = (prefix: string) => {
  const requestID = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `${prefix}-${requestID}`
}

export async function list(): Promise<ApiKeyPackage[]> {
  const { data } = await apiClient.get<ApiKeyPackage[]>('/key-packages')
  return data
}

export async function listPlans(): Promise<ApiKeyPackagePlan[]> {
  const { data } = await apiClient.get<ApiKeyPackagePlan[]>('/key-packages/plans')
  return data
}

export async function create(payload: SaveApiKeyPackageRequest): Promise<ApiKeyPackage> {
  const { data } = await apiClient.post<ApiKeyPackage>('/key-packages', payload, {
    headers: { 'Idempotency-Key': idempotencyKey('key-package-create') }
  })
  return data
}

export async function update(id: number, payload: SaveApiKeyPackageRequest): Promise<ApiKeyPackage> {
  const { data } = await apiClient.put<ApiKeyPackage>(`/key-packages/${id}`, payload)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/key-packages/${id}`)
}

export async function listKeys(
  packageId: number,
  page: number = 1,
  pageSize: number = 10,
): Promise<PaginatedResponse<ApiKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>(`/key-packages/${packageId}/keys`, {
    params: { page, page_size: pageSize }
  })
  return data
}

export async function listAvailableKeys(
  page: number = 1,
  pageSize: number = 100,
): Promise<PaginatedResponse<ApiKey>> {
  const { data } = await apiClient.get<PaginatedResponse<ApiKey>>('/key-packages/available-keys', {
    params: { page, page_size: pageSize }
  })
  return data
}

export async function bindKey(packageId: number, payload: BindPackageApiKeyRequest): Promise<ApiKey> {
  const { data } = await apiClient.post<ApiKey>(`/key-packages/${packageId}/keys`, payload, {
    headers: { 'Idempotency-Key': idempotencyKey(`key-package-${packageId}-key-bind`) }
  })
  return data
}

export async function upgradeKey(packageId: number, keyId: number, payload: UpgradePackageApiKeyRequest): Promise<ApiKey> {
  const { data } = await apiClient.post<ApiKey>(`/key-packages/${packageId}/keys/${keyId}/upgrade`, payload, {
    headers: { 'Idempotency-Key': idempotencyKey(`key-package-${packageId}-key-${keyId}-upgrade`) }
  })
  return data
}

export async function unbindKey(packageId: number, keyId: number): Promise<ApiKey> {
  const { data } = await apiClient.delete<ApiKey>(`/key-packages/${packageId}/keys/${keyId}`)
  return data
}

export const keyPackagesAPI = {
  list,
  listPlans,
  create,
  update,
  delete: remove,
  listKeys,
  listAvailableKeys,
  bindKey,
  upgradeKey,
  unbindKey,
}

export default keyPackagesAPI
