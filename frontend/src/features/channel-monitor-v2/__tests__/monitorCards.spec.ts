import { describe, expect, it } from 'vitest'
import { MONITOR_CARD_SLOT_COUNT, candyDisplayState, hasMonitorSamples, monitorCardTimeline } from '../monitorCards'
import type { MonitorCandyHistory, MonitorCoverage, MonitorMatrixRow, MonitorMetric } from '@/api/channelMonitorV2'

describe('monitor card data semantics', () => {
  it('does not infer successful traffic from redacted zero counters', () => {
    expect(hasMonitorSamples({ request_count: 0, has_samples: false } as MonitorMetric)).toBe(false)
    expect(hasMonitorSamples({ request_count: 0, has_samples: true } as MonitorMetric)).toBe(true)
    expect(hasMonitorSamples({ request_count: 3 } as MonitorMetric)).toBe(true)
  })
  it('keeps empty time windows unknown and uses the worst observed health in each window', () => {
    const metric = { request_count: 0, has_samples: true } as MonitorMetric
    const row = { buckets: [
      { bucket_start: '2026-09-28T00:06:00Z', metrics: metric, health: { overall: 'healthy' } },
      { bucket_start: '2026-09-28T00:07:00Z', metrics: metric, health: { overall: 'warning' } },
      { bucket_start: '2026-09-28T00:12:00Z', metrics: { ...metric, has_samples: false }, health: { overall: 'healthy' } },
    ] } as MonitorMatrixRow
    const bars = monitorCardTimeline(row, { requested_start: '2026-09-28T00:00:00Z', requested_end: '2026-09-28T01:30:00Z' } as MonitorCoverage)
    expect(bars).toHaveLength(MONITOR_CARD_SLOT_COUNT)
    expect(bars[0]).toMatchObject({ state: 'unknown', observed: false })
    expect(bars[1]).toMatchObject({ state: 'warning', observed: true })
    expect(bars[2]).toMatchObject({ state: 'unknown', observed: false })
    expect(monitorCardTimeline(row, undefined)).toEqual([])
  })
  it('keeps the latest dedicated samples even when they are outside the selected range', () => {
    const metric = { request_count: 1, has_samples: true } as MonitorMetric
    const sampleBuckets = Array.from({ length: 20 }, (_, index) => ({
      bucket_start: new Date(Date.UTC(2026, 8, 20, 0, index * 5)).toISOString(),
      metrics: metric,
      health: { overall: index === 19 ? 'critical' : 'healthy' },
    }))
    const row = {
      buckets: [{ bucket_start: '2026-09-28T00:06:00Z', metrics: metric, health: { overall: 'warning' } }],
      sample_buckets: sampleBuckets,
    } as MonitorMatrixRow
    const coverage = { requested_start: '2026-09-28T00:00:00Z', requested_end: '2026-09-28T01:30:00Z', bucket_seconds: 300 } as MonitorCoverage
    const samples = monitorCardTimeline(row, coverage, 'samples')
    expect(samples).toHaveLength(18)
    expect(samples[0].start).toBe(Date.parse(sampleBuckets[2].bucket_start))
    expect(samples.at(-1)?.state).toBe('critical')
    expect(samples[0].end - samples[0].start).toBe(5 * 60 * 1000)
    expect(samples.some((bar) => bar.start === Date.parse('2026-09-28T00:06:00Z'))).toBe(false)
    expect(monitorCardTimeline({ ...row, sample_buckets: [] }, coverage, 'samples')).toEqual([])
    expect(monitorCardTimeline(row, coverage)).toHaveLength(18)
  })
  it('distinguishes a wrong answer, failed probe, missing history and stale result', () => {
    const history: MonitorCandyHistory = { model: 'test', reasoning_effort: 'medium', interval_minutes: 1, results: [] }
    const now = Date.parse('2026-09-28T01:00:00Z')
    expect(candyDisplayState(history, now)).toBe('unknown')
    for (const verdict of ['correct', 'incorrect', 'error'] as const) {
      history.results = [{ checked_at: '2026-09-28T00:59:00Z', verdict, latency_ms: 3 }]
      expect(candyDisplayState(history, now)).toBe(verdict)
    }
    history.results[0].checked_at = '2026-09-28T00:55:00Z'
    expect(candyDisplayState(history, now)).toBe('stale')
  })
})
