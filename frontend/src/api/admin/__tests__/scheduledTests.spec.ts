import { beforeEach, describe, expect, it, vi } from 'vitest'
import { apiClient } from '../../client'
import { listPelicanHistory } from '../scheduledTests'

vi.mock('../../client', () => ({ apiClient: { get: vi.fn() } }))

beforeEach(() => {
  vi.mocked(apiClient.get).mockReset().mockResolvedValue({ data: { items: [], next_cursor: 0 } })
})

describe('Pelican history API', () => {
  it('asks for one account only when the records window names it', async () => {
    await listPelicanHistory(0, 24, 42)
    expect(apiClient.get).toHaveBeenLastCalledWith('/admin/pelican-test-results', { params: { before_id: 0, limit: 24, account_id: 42 } })
    await listPelicanHistory(9)
    expect(apiClient.get).toHaveBeenLastCalledWith('/admin/pelican-test-results', { params: { before_id: 9, limit: 24 } })
  })
})
