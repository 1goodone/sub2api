import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { adminAPI } from '@/api/admin'
import type { Account, AccountUsageStatsResponse } from '@/types'
import AccountStatsModal from '../AccountStatsModal.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}|${JSON.stringify(params)}` : key
    })
  }
})
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getStats: vi.fn() } } }))
vi.mock('vue-chartjs', () => ({
  Line: { name: 'ChartLine', props: ['data', 'options'], render: () => null }
}))

const BaseDialogStub = {
  props: ['show'],
  template: '<div v-if="show"><slot /><slot name="footer" /></div>'
}

const account = {
  id: 7,
  name: 'OpenAI Plus',
  platform: 'openai',
  type: 'apikey',
  credentials: {}
} as Account

const buildResponse = (days: number): AccountUsageStatsResponse => ({
  history: [
    {
      date: '2026-10-01',
      label: '10/01',
      requests: 3,
      tokens: 100,
      cost: 0.1,
      actual_cost: 0.1,
      user_cost: 0.1
    }
  ],
  summary: {
    days,
    actual_days_used: 1,
    total_cost: 0.1,
    total_user_cost: 0.1,
    total_standard_cost: 0.1,
    total_requests: 3,
    total_tokens: 100,
    avg_daily_cost: 0.1,
    avg_daily_user_cost: 0.1,
    avg_daily_requests: 3,
    avg_daily_tokens: 100,
    avg_duration_ms: 120,
    today: null,
    highest_cost_day: null,
    highest_request_day: null
  },
  models: [],
  endpoints: [],
  upstream_endpoints: []
})

const mountModal = () =>
  mount(AccountStatsModal, {
    props: { show: false, account },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Icon: true,
        LoadingSpinner: true,
        Line: true,
        ModelDistributionChart: true,
        EndpointDistributionChart: true,
        AccountOfficialStatsTab: true
      }
    }
  })

describe('AccountStatsModal local range filter', () => {
  beforeEach(() => {
    vi.mocked(adminAPI.accounts.getStats)
      .mockReset()
      .mockImplementation(async (_id: number, range) => buildResponse(range === '7d' ? 7 : 30))
  })

  it('loads the default 30d window when opened', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(adminAPI.accounts.getStats).toHaveBeenCalledWith(7, '30d')
    expect(wrapper.text()).toContain('admin.accounts.officialUsage.range30d')
  })

  it('reloads with the selected range and refresh keeps it', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    const rangeButtons = wrapper.findAll('button[data-range]')
    expect(rangeButtons.map((button) => button.attributes('data-range'))).toEqual([
      '7d',
      '30d',
      '90d',
      'all'
    ])

    await wrapper.find('button[data-range="7d"]').trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.getStats).toHaveBeenLastCalledWith(7, '7d')
    expect(wrapper.text()).toContain('admin.accounts.officialUsage.range7d')

    await wrapper.find('button[data-range="all"]').trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.getStats).toHaveBeenLastCalledWith(7, 'all')

    const refreshButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('admin.accounts.stats.refresh'))
    expect(refreshButton).toBeTruthy()
    await refreshButton!.trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.getStats).toHaveBeenLastCalledWith(7, 'all')
  })

  it('falls back to the default 30d window on reopen', async () => {
    const wrapper = mountModal()
    await wrapper.setProps({ show: true })
    await flushPromises()

    await wrapper.find('button[data-range="90d"]').trigger('click')
    await flushPromises()
    expect(adminAPI.accounts.getStats).toHaveBeenLastCalledWith(7, '90d')

    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()
    expect(adminAPI.accounts.getStats).toHaveBeenLastCalledWith(7, '30d')
  })
})
