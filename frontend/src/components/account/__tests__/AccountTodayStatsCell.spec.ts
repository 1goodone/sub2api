import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AccountTodayStatsCell from '../AccountTodayStatsCell.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

describe('AccountTodayStatsCell', () => {
  it('renders six metrics and recent call status bars', () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: {
        stats: {
          requests: 12,
          tokens: 2500,
          cost: 1.25,
          lifetime_requests: 345,
          lifetime_tokens: 8000000,
          lifetime_cost: 42,
          recent_calls: [
            { success: true, duration_ms: 128, created_at: '2026-09-30T01:00:00Z' },
            { success: false, error_message: 'rate limited', created_at: '2026-09-30T01:01:00Z' },
            { success: true, duration_ms: 845, created_at: '2026-09-30T01:02:00Z' }
          ]
        }
      }
    })

    expect(wrapper.get('[data-test="today-requests"]').text()).toBe('12')
    expect(wrapper.get('[data-test="lifetime-requests"]').text()).toBe('345')
    expect(wrapper.get('[data-test="today-tokens"]').text()).toBe('2.5K')
    expect(wrapper.get('[data-test="lifetime-tokens"]').text()).toBe('8.00M')
    expect(wrapper.get('[data-test="today-cost"]').text()).toContain('1.25')
    expect(wrapper.get('[data-test="lifetime-cost"]').text()).toContain('42.00')
    expect(wrapper.findAll('[data-test="recent-call-success"]')).toHaveLength(2)
    expect(wrapper.findAll('[data-test="recent-call-failure"]')).toHaveLength(1)
    expect(wrapper.findAll('[data-test="recent-call-success"]')[0].attributes('title')).toBe('128 ms')
    expect(wrapper.get('[data-test="recent-call-failure"]').attributes('title')).toBe('rate limited')
  })

  it('emits view-stats from the inline action', async () => {
    const wrapper = mount(AccountTodayStatsCell, {
      props: { stats: { requests: 0, tokens: 0, cost: 0 } }
    })

    await wrapper.get('[data-test="view-stats"]').trigger('click')

    expect(wrapper.emitted('view-stats')).toHaveLength(1)
  })
})
