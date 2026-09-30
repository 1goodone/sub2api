import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { adminAPI } from '@/api/admin'
import type { Account, AccountOfficialUsageResponse } from '@/types'
import AccountOfficialStatsTab from '../AccountOfficialStatsTab.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getOfficialUsage: vi.fn() } } }))

const account = { id: 7, name: 'OpenAI Plus', platform: 'openai', type: 'oauth' } as Account
const response: AccountOfficialUsageResponse = {
  state: 'ok',
  range: '30d',
  days: 30,
  source: 'chatgpt-wham',
  fetched_at: '2026-09-30T12:00:00Z',
  stale: false,
  breakdown_state: 'ok',
  retention_days: 84,
  credits_per_usd: 25,
  totals: { credits: 50, usd: 2, tokens: 1200, turns: 8, threads: 4, settled_days: 1 },
  cycle: {
    available: true,
    reliable: true,
    utilization: 20,
    used_usd: 2,
    estimated_usd: 10,
    estimated_low_usd: 9.75,
    estimated_high_usd: 10.26,
    resets_at: '2026-10-03T12:00:00Z'
  },
  daily: [
    {
      date: '2026-09-29',
      settled: true,
      credits: 50,
      usd: 2,
      users: 1,
      threads: 4,
      turns: 8,
      uncached_input_tokens: 200,
      cached_input_tokens: 400,
      output_tokens: 600,
      total_tokens: 1200
    }
  ],
  clients: [{ client_id: 'CODEX_CLI', credits: 50, usd: 2, tokens: 1200, turns: 8 }],
  models: [{ model: 'gpt-test', speed: 'standard', credits: 50, usd: 2 }]
}

describe('AccountOfficialStatsTab', () => {
  beforeEach(() => {
    vi.mocked(adminAPI.accounts.getOfficialUsage).mockReset().mockResolvedValue(response)
  })

  it('loads lazily, renders daily bars, and supports range refresh', async () => {
    const wrapper = mount(AccountOfficialStatsTab, {
      props: { account },
      global: { stubs: { Icon: true, LoadingSpinner: true } }
    })
    await flushPromises()

    expect(adminAPI.accounts.getOfficialUsage).toHaveBeenCalledWith(7, '30d', false)
    expect(wrapper.text()).toContain('$2.00')
    expect(wrapper.text()).toContain('1200 tok')
    expect(wrapper.findAll('div').some((node) => node.attributes('class')?.includes('bg-emerald-500/80'))).toBe(true)

    const rangeButton = wrapper.findAll('button').find((button) => button.text().includes('range7d'))
    expect(rangeButton).toBeTruthy()
    await rangeButton!.trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.getOfficialUsage).toHaveBeenLastCalledWith(7, '7d', false)

    const refreshButton = wrapper.findAll('button').find((button) => button.text().includes('refresh'))
    await refreshButton!.trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.getOfficialUsage).toHaveBeenLastCalledWith(7, '7d', true)
  })
})
