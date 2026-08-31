import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { UserMonitorView } from '@/api/channelMonitor'

const { listMonitors } = vi.hoisted(() => ({ listMonitors: vi.fn() }))

vi.mock('@/api/channelMonitor', async () => {
  const actual = await vi.importActual<typeof import('@/api/channelMonitor')>('@/api/channelMonitor')
  return { ...actual, list: listMonitors, status: vi.fn() }
})

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    cachedPublicSettings: { channel_monitor_enabled: true },
    showError: vi.fn(),
  }),
}))

vi.mock('@/composables/useAutoRefresh', () => ({
  useAutoRefresh: () => ({
    enabled: ref(true),
    intervalSeconds: ref(60),
    countdown: ref(60),
    intervals: [30, 60, 120],
    setEnabled: vi.fn(),
    setInterval: vi.fn(),
    start: vi.fn(),
    stop: vi.fn(),
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key, te: () => true }) }
})

import ChannelStatusV1View from '../ChannelStatusV1View.vue'

function monitor(id: number, status: UserMonitorView['primary_status']): UserMonitorView {
  return {
    id,
    name: `channel-${id}`,
    provider: 'openai',
    group_name: '',
    primary_model: 'gpt-5',
    primary_status: status,
    primary_latency_ms: 500,
    primary_ping_latency_ms: 40,
    availability_7d: 99,
    extra_models: [],
    timeline: [],
  }
}

describe('ChannelStatusV1View production flow', () => {
  beforeEach(() => {
    listMonitors.mockResolvedValue({
      items: [monitor(1, 'operational'), monitor(2, 'failed'), monitor(3, 'degraded')],
    })
  })

  it('renders API data by severity and keeps one production row expanded', async () => {
    const wrapper = mount(ChannelStatusV1View, {
      global: {
        stubs: {
          AppLayout: defineComponent({ setup: (_, { slots }) => () => h('main', slots.default?.()) }),
          MonitorHero: true,
          MonitorDetailDialog: true,
        },
      },
    })
    await flushPromises()

    expect(listMonitors).toHaveBeenCalledOnce()
    expect(wrapper.findAll('[data-testid^="monitor-row-"]').map(row => row.attributes('data-testid'))).toEqual([
      'monitor-row-2',
      'monitor-row-3',
      'monitor-row-1',
    ])

    await wrapper.get('[data-testid="monitor-row-2"] > button').trigger('click')
    await wrapper.get('[data-testid="monitor-row-3"] > button').trigger('click')

    expect(wrapper.findAll('[data-testid="monitor-expanded-panel"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="monitor-row-2"] > button').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-testid="monitor-row-3"] > button').attributes('aria-expanded')).toBe('true')
  })
})
