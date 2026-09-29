import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpenAIAccountSummaryCards from '../OpenAIAccountSummaryCards.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, values?: { estimated: number; total: number }) =>
    values ? `Estimated ${values.estimated} / ${values.total} accounts` : key })
}))

const summary = {
  accounts: { oauth: 8, apikey: 3 },
  oauth_status: { normal: 4, rate_limited: 3, error: 1 },
  oauth_quota: {
    '5h': { remaining_cost: 5, estimated_cost: 10, estimated_accounts: 3, total_accounts: 8 },
    '7d': { remaining_cost: 42.5, estimated_cost: 80, estimated_accounts: 5, total_accounts: 8 }
  },
  concurrency: { oauth: { used: 3, total: 8 }, apikey: { used: 1, total: 4 } }
}

describe('OpenAIAccountSummaryCards', () => {
  it('renders full-pool values and emits the clicked quota filter', async () => {
    const wrapper = mount(OpenAIAccountSummaryCards, {
      props: { summary, filters: { type: 'oauth', quota_window: '5h' } }
    })

    expect(wrapper.get('[data-test="openai-summary-accounts-oauth"]').text()).toContain('8')
    expect(wrapper.get('[data-test="openai-summary-concurrency-oauth"]').text()).toContain('3/8')
    expect(wrapper.get('[data-test="openai-summary-quota-5h"]').text()).toContain('$5.00 / $10.00')
    expect(wrapper.get('[data-test="openai-summary-quota-5h"]').text()).toContain('Estimated 3 / 8 accounts')
    expect(wrapper.get('[data-test="openai-summary-quota-7d"]').text()).toContain('Estimated 5 / 8 accounts')
    expect(wrapper.get('[data-test="openai-summary-quota-5h"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-test="openai-summary-quota-7d"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([{ type: 'oauth', quota_window: '7d' }])
  })

  it('distinguishes unknown quota, loading, and known zero remaining', () => {
    const wrapper = mount(OpenAIAccountSummaryCards, {
      props: { summary: { ...summary, oauth_quota: {
        '5h': { remaining_cost: null, estimated_cost: null, estimated_accounts: 0, total_accounts: 8 },
        '7d': { remaining_cost: 0, estimated_cost: 80, estimated_accounts: 5, total_accounts: 8 }
      } }, filters: {} }
    })
    expect(wrapper.get('[data-test="openai-summary-quota-5h"]').text()).toContain('pendingEstimate')
    expect(wrapper.get('[data-test="openai-summary-quota-5h"]').text()).toContain('Estimated 0 / 8 accounts')
    expect(wrapper.get('[data-test="openai-summary-quota-7d"]').text()).toContain('$0.00 / $80.00')
    const loading = mount(OpenAIAccountSummaryCards, { props: { summary: null, filters: {} } })
    expect(loading.get('[data-test="openai-summary-quota-5h"]').text()).toContain('—')
    expect(loading.text()).not.toContain('Estimated 0 / 0')
  })
})
