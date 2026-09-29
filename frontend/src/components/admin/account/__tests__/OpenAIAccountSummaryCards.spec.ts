import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OpenAIAccountSummaryCards from '../OpenAIAccountSummaryCards.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))

const summary = {
  accounts: { oauth: 8, apikey: 3 },
  oauth_status: { normal: 4, rate_limited: 3, error: 1 },
  oauth_quota: {
    '5h': { remaining_cost: 5, estimated_cost: 10 },
    '7d': { remaining_cost: 42.5, estimated_cost: 80 }
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
    expect(wrapper.get('[data-test="openai-summary-quota-5h"]').attributes('aria-pressed')).toBe('true')

    await wrapper.get('[data-test="openai-summary-quota-7d"]').trigger('click')
    expect(wrapper.emitted('select')?.[0]).toEqual([{ type: 'oauth', quota_window: '7d' }])
  })
})
