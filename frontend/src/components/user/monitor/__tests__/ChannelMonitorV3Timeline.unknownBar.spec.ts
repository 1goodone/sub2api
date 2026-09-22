import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { MonitorHealth, MonitorMatrixBucket, MonitorMetric } from '@/api/channelMonitorV2'
import ChannelMonitorV3Timeline from '../ChannelMonitorV3Timeline.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key, locale: { value: 'zh-CN' } }),
  }
})

function metrics(errorRate: number, cacheRate = 0): MonitorMetric {
  return {
    success_requests: 0,
    error_requests: 0,
    request_count: 0,
    token_count: 0,
    rpm: 0,
    tpm: 0,
    error_rate: errorRate,
    cache_rate: cacheRate,
    cache_rate_numerator: cacheRate * 100,
    cache_rate_denominator: 100,
    ttft: { sample_count: 0, p50_ms: null, p95_ms: null, avg_ms: null },
    duration: { sample_count: 0, p50_ms: null, p95_ms: null, avg_ms: null },
  }
}

function health(overall: MonitorHealth['overall'], errorRate = overall): MonitorHealth {
  return {
    overall,
    error_rate: errorRate,
    ttft: overall,
    cache: overall,
    minimum_sample: 50,
  }
}

function bucket(
  overall: MonitorHealth['overall'],
  errorRate: number,
  start: string,
  cacheRate = 0,
  errorHealth: MonitorHealth['error_rate'] = overall,
): MonitorMatrixBucket {
  return {
    bucket_start: start,
    metrics: metrics(errorRate, cacheRate),
    health: health(overall, errorHealth),
  }
}

describe('ChannelMonitorV3Timeline unknown bars', () => {
  it('paints insufficient-sample bars gray instead of availability black', () => {
    const wrapper = mount(ChannelMonitorV3Timeline, {
      props: {
        buckets: [bucket('unknown', 1, '2026-09-11T12:00:00Z')],
        countdownSeconds: 0,
        length: 1,
      },
    })
    const bar = wrapper.get('.v3-soft-glass-bar')
    expect(bar.classes()).toContain('bg-gray-300')
    expect(bar.classes()).not.toContain('bg-gray-950')
    expect(bar.attributes('style')).toContain('height: 15%')
  })

  it('keeps true low availability on the black band', () => {
    const wrapper = mount(ChannelMonitorV3Timeline, {
      props: {
        buckets: [bucket('critical', 0.8, '2026-09-11T12:00:00Z')],
        countdownSeconds: 0,
        length: 1,
      },
    })
    const bar = wrapper.get('.v3-soft-glass-bar')
    expect(bar.classes()).toContain('bg-gray-950')
    expect(bar.attributes('style')).toContain('height: 20%')
  })

  it('uses only error rate for height even when cache makes overall health critical', () => {
    const wrapper = mount(ChannelMonitorV3Timeline, {
      props: {
        buckets: [bucket('critical', 0, '2026-09-11T12:00:00Z', 0.01, 'healthy')],
        countdownSeconds: 0,
        length: 1,
      },
    })
    const bar = wrapper.get('.v3-soft-glass-bar')
    expect(bar.classes()).toContain('bg-emerald-600')
    expect(bar.attributes('style')).toContain('height: 100%')
  })

  it('reduces height linearly with error rate', () => {
    const wrapper = mount(ChannelMonitorV3Timeline, {
      props: {
        buckets: [bucket('warning', 0.25, '2026-09-11T12:00:00Z', 0.9, 'warning')],
        countdownSeconds: 0,
        length: 1,
      },
    })
    expect(wrapper.get('.v3-soft-glass-bar').attributes('style')).toContain('height: 75%')
  })
})
