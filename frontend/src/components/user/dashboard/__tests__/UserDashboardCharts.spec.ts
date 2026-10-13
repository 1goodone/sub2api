import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import UserDashboardCharts from '../UserDashboardCharts.vue'

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('model distribution shares', () => {
  const render = (tokens: number[]) => mount(UserDashboardCharts, {
    props: {
      loading: false, startDate: '2026-10-01', endDate: '2026-10-13',
      granularity: 'day', trend: [],
      models: tokens.map((total_tokens, index) => ({
        model: `model-${index}`, total_tokens, requests: 1, actual_cost: 0.1, cost: 0.2,
      })),
    },
    global: { stubs: { TokenUsageTrend: true, DateRangePicker: true, Select: true, Icon: true } },
  })

  it('shows token-weighted shares and preserves standard cost details', () => {
    const wrapper = render([75, 25])
    expect(wrapper.findAll('[role="meter"]').map(node => node.attributes('aria-valuenow'))).toEqual(['75', '25'])
    expect(wrapper.find('details').text()).toContain('$0.2000')
  })

  it('keeps zero-token shares finite and renders the empty state', () => {
    expect(render([0]).find('[role="meter"]').attributes('aria-valuenow')).toBe('0')
    const empty = render([])
    expect(empty.find('[role="meter"]').exists()).toBe(false)
    expect(empty.text()).toContain('dashboard.noDataAvailable')
  })
})
