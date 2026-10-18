import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import UsageHeatmap from '../UsageHeatmap.vue'
import en from '@/i18n/locales/en'

vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({
    locale: ref('en'),
    t: (key: string) => key.split('.').reduce<any>((value, segment) => value?.[segment], en) ?? key,
  }),
}))

const point = (date: string, tokens: number, requests: number, cost: number) => ({
  date, total_tokens: tokens, requests, actual_cost: cost, cost,
  input_tokens: tokens, output_tokens: 0, cache_creation_tokens: 0, cache_read_tokens: 0,
})
function render(data = [point('2026-10-12', 100, 2, 1), point('2026-10-13', 25, 8, 4)], startDate = '2025-10-13', endDate = '2026-10-13') {
  return mount(UsageHeatmap, {
    props: { data, startDate, endDate },
    global: { stubs: { Icon: true } },
  })
}
describe('UsageHeatmap', () => {
  it('renders exactly 53 calendar weeks, leaves future days disabled and selects a real date', async () => {
    const wrapper = render()
    expect(wrapper.findAll('.heatmap-day')).toHaveLength(371)
    expect(wrapper.find('[data-date="2026-10-14"]').attributes('disabled')).toBeDefined()
    const day = wrapper.find('[data-date="2026-10-13"]')
    expect(day.attributes('aria-label')).toContain('25 Tokens')
    await day.trigger('click')
    expect(wrapper.emitted('select')).toEqual([['2026-10-13']])
    wrapper.unmount()
  })
  it('switches both values and color intensity between tokens, requests and billed costs', async () => {
    const wrapper = render()
    const day = wrapper.find('[data-date="2026-10-13"]')
    expect(day.classes()).toContain('level-1')
    await wrapper.findAll('.heatmap-metrics button')[1].trigger('click')
    expect(day.classes()).toContain('level-4')
    expect(day.attributes('aria-label')).toContain('8 Requests')
    await wrapper.findAll('.heatmap-metrics button')[2].trigger('click')
    expect(day.attributes('aria-label')).toContain('$4.0000')
    expect(wrapper.find('.heatmap-summary').text()).toContain('$5.00')
    wrapper.unmount()
  })
  it('uses one calendar tab stop and moves focus by day or week without entering future dates', async () => {
    const wrapper = mount(UsageHeatmap, {
      props: { data: [], startDate: '2025-10-13', endDate: '2026-10-13' },
      attachTo: document.body,
      global: { stubs: { Icon: true } },
    })
    try {
      expect(wrapper.findAll('.heatmap-day[tabindex="0"]')).toHaveLength(1)
      const today = wrapper.find<HTMLButtonElement>('[data-date="2026-10-13"]')
      today.element.focus()
      await today.trigger('keydown', { key: 'ArrowDown' })
      expect(document.activeElement).toBe(today.element)
      await today.trigger('keydown', { key: 'ArrowLeft' })
      expect((document.activeElement as HTMLElement).dataset.date).toBe('2026-10-06')
      await wrapper.find('[data-date="2026-10-06"]').trigger('keydown', { key: 'ArrowUp' })
      expect((document.activeElement as HTMLElement).dataset.date).toBe('2026-10-05')
      expect(wrapper.findAll('.heatmap-day[tabindex="0"]')).toHaveLength(1)
    } finally {
      wrapper.unmount()
    }
  })
  it('shows date, billed spending and exact total tokens on hover or focus for every metric, including empty days', async () => {
    const wrapper = render()
    const day = wrapper.find('[data-date="2026-10-13"]')
    await day.trigger('mouseenter')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('2026-10-13')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('Total Cost$4.0000')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('Total Tokens25')
    expect(day.attributes('aria-describedby')).toBe('usage-heatmap-day-tooltip')
    await wrapper.findAll('.heatmap-metrics button')[1].trigger('click')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('$4.0000')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('Total Tokens25')
    await day.trigger('mouseleave')
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    const empty = wrapper.find('[data-date="2026-10-11"]')
    await empty.trigger('focus')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('2026-10-11')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('$0.0000')
    expect(wrapper.find('[role="tooltip"]').text()).toContain('Total Tokens0')
    await empty.trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('closes the tooltip when scrolling or resizing the viewport', async () => {
    const wrapper = render()
    const day = wrapper.find('[data-date="2026-10-13"]')
    for (const event of ['scroll', 'resize']) {
      await day.trigger('focus')
      expect(wrapper.find('[role="tooltip"]').exists()).toBe(true)
      window.dispatchEvent(new Event(event))
      await wrapper.vm.$nextTick()
      expect(wrapper.find('[role="tooltip"]').exists()).toBe(false)
    }
    wrapper.unmount()
  })
  it('does not count data outside the period and displays an empty calendar', () => {
    const wrapper = render([point('2025-10-12', 999, 999, 999)])
    expect(wrapper.find('.heatmap-summary').text()).toContain('0 Tokens')
    expect(wrapper.findAll('.level-4.heatmap-day')).toHaveLength(0)
    expect(wrapper.text()).toContain('No usage in this period')
    wrapper.unmount()
  })
  it('includes leap day exactly once without shifting weekdays', () => {
    const wrapper = render([point('2024-02-29', 10, 1, 0.1)], '2023-03-06', '2024-03-03')
    const days = wrapper.findAll('.heatmap-day')
    const leapIndex = days.findIndex((day) => day.attributes('data-date') === '2024-02-29')
    expect(leapIndex % 7).toBe(3)
    expect(days.filter((day) => day.attributes('data-date') === '2024-02-29')).toHaveLength(1)
    expect(days[leapIndex + 1].attributes('data-date')).toBe('2024-03-01')
    wrapper.unmount()
  })
  it('disables selection while loading and shows retry instead of a misleading empty chart on failure', async () => {
    const wrapper = render()
    const day = wrapper.find('[data-date="2026-10-13"]').element
    const summary = wrapper.find('.heatmap-summary').text()
    await wrapper.setProps({ loading: true })
    expect(wrapper.find('[data-date="2026-10-13"]').element).toBe(day)
    expect(wrapper.find('.heatmap-summary').text()).toBe(summary)
    expect(wrapper.find('.heatmap-calendar').classes()).not.toContain('is-loading')
    expect(wrapper.find('[data-date="2026-10-13"]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ loading: false, error: true })
    expect(wrapper.find('.heatmap-days').exists()).toBe(false)
    await wrapper.find('.heatmap-error button').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    wrapper.unmount()
  })
})
