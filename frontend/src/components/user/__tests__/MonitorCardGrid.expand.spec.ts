import { defineComponent, ref } from 'vue'
import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import type { UserMonitorView } from '@/api/channelMonitor'
import MonitorCardGrid from '../monitor/MonitorCardGrid.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, te: () => true }),
  }
})

vi.mock('@/utils/featureFlags', () => ({
  isChannelMonitorQuotaVisible: () => false,
}))

function item(id: number): UserMonitorView {
  return {
    id,
    name: `channel-${id}`,
    provider: 'openai',
    group_name: '',
    primary_model: 'gpt-5',
    primary_status: 'operational',
    primary_latency_ms: 300,
    primary_ping_latency_ms: 30,
    availability_7d: 99,
    extra_models: [],
    timeline: [],
  }
}

describe('MonitorCardGrid expandable rows', () => {
  it('keeps only the selected row expanded', async () => {
    const Harness = defineComponent({
      components: { MonitorCardGrid },
      setup() {
        const expandedId = ref<number | null>(null)
        return { expandedId, items: [item(1), item(2)] }
      },
      template: `
        <MonitorCardGrid
          :items="items"
          window="7d"
          :countdown-seconds="30"
          :loading="false"
          :detail-cache="{}"
          :expanded-id="expandedId"
          @toggle="expandedId = expandedId === $event ? null : $event"
        />
      `,
    })
    const wrapper = mount(Harness)

    await wrapper.get('[data-testid="monitor-row-1"] > button').trigger('click')
    expect(wrapper.findAll('[data-testid="monitor-expanded-panel"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="monitor-row-1"] > button').attributes('aria-expanded')).toBe('true')

    await wrapper.get('[data-testid="monitor-row-2"] > button').trigger('click')
    expect(wrapper.findAll('[data-testid="monitor-expanded-panel"]')).toHaveLength(1)
    expect(wrapper.get('[data-testid="monitor-row-1"] > button').attributes('aria-expanded')).toBe('false')
    expect(wrapper.get('[data-testid="monitor-row-2"] > button').attributes('aria-expanded')).toBe('true')
  })
})
