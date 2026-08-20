import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post, put } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post, put }
}))

import {
  getUpstreamBillingProbeSettings,
  probeUpstreamBilling,
  probeUpstreamBillingBatch,
  setUpstreamBillingProbeEnabled,
  updateUpstreamBillingProbeSettings,
  setCodexQuotaOverdraftEnabled
} from '@/api/admin/accounts'

describe('admin account upstream billing probe API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
    put.mockReset()
  })

  it('reads and updates global settings', async () => {
    const settings = { enabled: true, interval_minutes: 30 }
    get.mockResolvedValueOnce({ data: settings })
    put.mockResolvedValueOnce({ data: settings })

    await expect(getUpstreamBillingProbeSettings()).resolves.toEqual(settings)
    await expect(updateUpstreamBillingProbeSettings(settings)).resolves.toEqual(settings)
    expect(get).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings')
    expect(put).toHaveBeenCalledWith('/admin/accounts/upstream-billing-probe/settings', settings)
  })

  it('uses dedicated account and batch endpoints', async () => {
    const result = { account_id: 7, snapshot: { status: 'unsupported' } }
    put.mockResolvedValueOnce({ data: {} })
    post.mockResolvedValueOnce({ data: result })
    post.mockResolvedValueOnce({ data: { results: [result] } })

    await setUpstreamBillingProbeEnabled(7, true)
    await expect(probeUpstreamBilling(7)).resolves.toEqual(result)
    await expect(probeUpstreamBillingBatch([7])).resolves.toEqual([result])

    expect(put).toHaveBeenCalledWith('/admin/accounts/7/upstream-billing-probe', { enabled: true })
    expect(post).toHaveBeenNthCalledWith(1, '/admin/accounts/7/upstream-billing-probe')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/accounts/upstream-billing-probe/batch', { account_ids: [7] })
  })

  it('uses the per-account Codex overdraft endpoint', async () => {
    put.mockResolvedValueOnce({ data: { id: 8, extra: { codex_quota_overdraft_enabled: true } } })

    await expect(setCodexQuotaOverdraftEnabled(8, true)).resolves.toMatchObject({ id: 8 })
    expect(put).toHaveBeenCalledWith('/admin/accounts/8/codex-quota-overdraft', { enabled: true })
  })
})
