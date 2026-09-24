import { describe, expect, it } from 'vitest'
import type { MonitorHealth, MonitorMatrixBucket, MonitorMetric } from '@/api/channelMonitorV2'
import { alignV3TimelineSlots, listV3TimelineSlotStarts } from '../v3TimelineSlots'

function metrics(): MonitorMetric {
  return {
    success_requests: 1,
    error_requests: 0,
    request_count: 1,
    token_count: 1,
    rpm: 1,
    tpm: 1,
    error_rate: 0,
    cache_rate: 0,
    cache_rate_numerator: 0,
    cache_rate_denominator: 1,
    ttft: { sample_count: 1, p50_ms: 100, p95_ms: 200, avg_ms: 120 },
    duration: { sample_count: 1, p50_ms: 300, p95_ms: 400, avg_ms: 320 },
  }
}

function health(): MonitorHealth {
  return {
    overall: 'healthy',
    error_rate: 'healthy',
    ttft: 'healthy',
    cache: 'healthy',
    minimum_sample: 1,
  }
}

function bucket(start: string): MonitorMatrixBucket {
  return { bucket_start: start, metrics: metrics(), health: health() }
}

describe('v3TimelineSlots', () => {
  const coverage = {
    requested_start: '2026-09-24T05:05:00.000Z',
    bucket_seconds: 300,
  }

  it('emits a fixed-length clock grid from requested_start', () => {
    expect(listV3TimelineSlotStarts(coverage, 18)).toEqual([
      '2026-09-24T05:05:00.000Z',
      '2026-09-24T05:10:00.000Z',
      '2026-09-24T05:15:00.000Z',
      '2026-09-24T05:20:00.000Z',
      '2026-09-24T05:25:00.000Z',
      '2026-09-24T05:30:00.000Z',
      '2026-09-24T05:35:00.000Z',
      '2026-09-24T05:40:00.000Z',
      '2026-09-24T05:45:00.000Z',
      '2026-09-24T05:50:00.000Z',
      '2026-09-24T05:55:00.000Z',
      '2026-09-24T06:00:00.000Z',
      '2026-09-24T06:05:00.000Z',
      '2026-09-24T06:10:00.000Z',
      '2026-09-24T06:15:00.000Z',
      '2026-09-24T06:20:00.000Z',
      '2026-09-24T06:25:00.000Z',
      '2026-09-24T06:30:00.000Z',
    ])
  })

  it('keeps empty windows in their clock slots instead of left-padding', () => {
    const slots = alignV3TimelineSlots(
      [
        bucket('2026-09-24T05:05:00Z'),
        bucket('2026-09-24T06:30:00Z'),
      ],
      coverage,
      18,
    )
    expect(slots).toHaveLength(18)
    expect(slots[0]?.bucket?.bucket_start).toBe('2026-09-24T05:05:00Z')
    expect(slots[1]?.bucket).toBeUndefined()
    expect(slots[9]?.start).toBe('2026-09-24T05:50:00.000Z')
    expect(slots[9]?.bucket).toBeUndefined()
    expect(slots[17]?.bucket?.bucket_start).toBe('2026-09-24T06:30:00Z')
  })

  it('left-pads when coverage is missing so older payloads still render', () => {
    const slots = alignV3TimelineSlots(
      [bucket('2026-09-24T06:30:00Z')],
      null,
      4,
    )
    expect(slots.map(slot => slot.bucket?.bucket_start ?? '')).toEqual([
      '',
      '',
      '',
      '2026-09-24T06:30:00Z',
    ])
  })
})
