import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { MonitorV3StatusPage } from '@/api/channelMonitorV3'

const getStatus = vi.fn()

vi.mock('@/api/channelMonitorV3', () => ({
  getStatus: (...args: unknown[]) => getStatus(...args),
}))
vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: () => 'load failed',
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<div><slot /></div>' },
}))
vi.mock('@/features/channel-monitor-v3/StatusPage.vue', () => ({
  default: defineComponent({
    name: 'StatusPage',
    props: ['status', 'loading'],
    emits: ['refresh', 'navigate', 'open-incidents'],
    template: `
      <div data-testid="status-page" :data-loading="String(loading)">
        <span data-testid="status-generated">{{ status?.generated_at }}</span>
        <button data-testid="navigate" @click="$emit('navigate', 123)">navigate</button>
        <button data-testid="refresh" @click="$emit('refresh')">refresh</button>
        <button data-testid="incidents" @click="$emit('open-incidents')">incidents</button>
      </div>
    `,
  }),
}))
vi.mock('@/features/channel-monitor-v3/IncidentsDialog.vue', () => ({
  default: defineComponent({
    name: 'IncidentsDialog',
    props: ['show', 'admin'],
    template: '<div data-testid="incidents-dialog" :data-show="String(show)" />',
  }),
}))

import ChannelStatusV3View from '../ChannelStatusV3View.vue'

function page(latest = true): MonitorV3StatusPage {
  return {
    generated_at: latest ? '2026-10-07T00:00:00Z' : '2026-10-06T00:00:00Z',
    interval_minutes: 5,
    cells: 24,
    availability_range: '24h',
    availability_since: '2026-10-06T00:00:00Z',
    down_error_rate: 0.2,
    degraded_error_rate: 0.05,
    degraded_ttft_ms: 10000,
    min_requests: 3,
    data_through: null,
    footer_note: '',
    window: { start: '2026-10-06T22:00:00Z', end: '2026-10-07T00:00:00Z', latest, has_older: true },
    featured: null,
    categories: [],
    open_incidents: 0,
  }
}

describe('ChannelStatusV3View', () => {
  beforeEach(() => {
    getStatus.mockReset()
  })

  it('loads the component status page and navigates historical windows', async () => {
    getStatus.mockResolvedValueOnce(page()).mockResolvedValueOnce(page(false))
    const wrapper = mount(ChannelStatusV3View)
    await flushPromises()

    expect(getStatus).toHaveBeenNthCalledWith(1, null, false, expect.anything())
    expect(wrapper.get('[data-testid="status-generated"]').text()).toBe('2026-10-07T00:00:00Z')

    await wrapper.get('[data-testid="navigate"]').trigger('click')
    await flushPromises()
    expect(getStatus).toHaveBeenNthCalledWith(2, 123, false, expect.anything())
    expect(wrapper.get('[data-testid="status-generated"]').text()).toBe('2026-10-06T00:00:00Z')
    wrapper.unmount()
  })

  it('refreshes the selected window and opens incident history', async () => {
    getStatus.mockResolvedValueOnce(page(false)).mockResolvedValueOnce(page(false))
    const wrapper = mount(ChannelStatusV3View)
    await flushPromises()

    await wrapper.get('[data-testid="navigate"]').trigger('click')
    await flushPromises()
    getStatus.mockResolvedValueOnce(page(false))
    await wrapper.get('[data-testid="refresh"]').trigger('click')
    await flushPromises()
    expect(getStatus).toHaveBeenLastCalledWith(123, false, expect.anything())

    await wrapper.get('[data-testid="incidents"]').trigger('click')
    expect(wrapper.get('[data-testid="incidents-dialog"]').attributes('data-show')).toBe('true')
    wrapper.unmount()
  })

  it('shows a readable load error', async () => {
    getStatus.mockRejectedValueOnce(new Error('boom'))
    const wrapper = mount(ChannelStatusV3View)
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toBe('load failed')
    expect(wrapper.get('[data-testid="status-page"]').attributes('data-loading')).toBe('false')
    wrapper.unmount()
  })
})
