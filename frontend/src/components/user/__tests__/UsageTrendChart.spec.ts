import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import UsageTrendChart from '../UsageTrendChart.vue'
import en from '@/i18n/locales/en'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ locale: ref('en'), t: (key: string) => key.split('.').reduce<any>((value, segment) => value?.[segment], en) ?? key }),
}))
vi.mock('vue-chartjs', () => ({
  Bar: { name: 'Bar', props: ['data', 'options'], template: '<div data-testid="bar-data">{{ JSON.stringify(data) }}</div>' },
}))
const point = (date: string, tokens: number, requests: number, cost: number) => ({
  date, total_tokens: tokens, requests, actual_cost: cost, cost,
  input_tokens: tokens, output_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0,
})
const render = (data = [point('2026-10-13 10:00', 100, 2, 0.05)]) => mount(UsageTrendChart, {
  props: { data, startDate: '2026-10-13', endDate: '2026-10-13', granularity: 'hour' },
  global: { stubs: { LoadingSpinner: true, Icon: true } },
})
const chartData = (wrapper: ReturnType<typeof render>) => JSON.parse(wrapper.find('[data-testid="bar-data"]').text())

describe('UsageTrendChart', () => {
  it('keeps all 24 hours ordered, fills gaps with zero and sums duplicate buckets', () => {
    const wrapper = render([
      point('2026-10-13 11:00', 200, 3, 0.2),
      point('2026-10-13 10:00', 100, 2, 0.1),
      point('2026-10-13T10:00:00', 50, 1, 0.05),
      point('2026-10-12 10:00', 9999, 999, 99),
    ])
    const data = chartData(wrapper)
    expect(data.labels).toHaveLength(24)
    expect(data.labels[0]).toBe('10/13 00:00')
    expect(data.labels[23]).toBe('10/13 23:00')
    expect(data.datasets[0].data.slice(9, 13)).toEqual([0, 150, 200, 0])
    expect(wrapper.findAll('tbody tr')).toHaveLength(24)
    wrapper.unmount()
  })

  it('switches tokens, request counts and actual charges without changing the hour slots', async () => {
    const wrapper = render()
    await wrapper.findAll('.usage-trend-metrics button')[1].trigger('click')
    expect(chartData(wrapper).datasets[0].data[10]).toBe(2)
    expect(chartData(wrapper).datasets[0].label).toBe('Requests')
    await wrapper.findAll('.usage-trend-metrics button')[2].trigger('click')
    expect(chartData(wrapper).datasets[0].data[10]).toBe(0.05)
    expect(wrapper.findAll('tbody tr')[10].text()).toContain('$0.0500')
    wrapper.unmount()
  })

  it('uses daily buckets for a multi-day date range', async () => {
    const wrapper = render([point('2026-10-13', 100, 2, 0.05), point('2026-10-15', 200, 3, 0.1)])
    await wrapper.setProps({ granularity: 'day', endDate: '2026-10-15' })
    expect(chartData(wrapper).labels).toEqual(['10/13', '10/14', '10/15'])
    expect(chartData(wrapper).datasets[0].data).toEqual([100, 0, 200])
    wrapper.unmount()
  })

  it('shows loading, empty and retry states without drawing obsolete bars', async () => {
    const wrapper = render()
    await wrapper.setProps({ loading: true })
    expect(wrapper.find('[data-testid="bar-data"]').exists()).toBe(false)
    await wrapper.setProps({ loading: false, error: true })
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    await wrapper.find('.usage-trend-state button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    await wrapper.setProps({ error: false, data: [] })
    expect(wrapper.text()).toContain('No usage in this period')
    wrapper.unmount()
  })
})
