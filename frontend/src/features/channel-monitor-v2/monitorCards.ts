import type { HealthState, MonitorCandyHistory, MonitorCoverage, MonitorMatrixRow, MonitorMetric } from '@/api/channelMonitorV2'

export const MONITOR_CARD_SLOT_COUNT = 18
export const CANDY_HISTORY_LIMIT = 100
export const CANDY_HISTORY_LIMIT_MIN = 15
export const CANDY_HISTORY_LIMIT_MAX = 100

export type MonitorCardTimelineMode = 'windows' | 'samples'

export function hasMonitorSamples(metric: MonitorMetric): boolean {
  return metric.has_samples === true || metric.request_count > 0
}

export function monitorRefreshSeconds(configSeconds: number | undefined, rows: readonly MonitorMatrixRow[], bootstrap: boolean): number {
  if (bootstrap) return 10
  let seconds = configSeconds && Number.isFinite(configSeconds) && configSeconds > 0 ? configSeconds : 300
  for (const row of rows) {
    const interval = row.candy?.interval_minutes
    if (interval && Number.isFinite(interval) && interval > 0) seconds = Math.min(seconds, interval * 60)
  }
  return Math.max(60, seconds)
}

export interface MonitorCardBar {
  start: number
  end: number
  state: HealthState
  observed: boolean
}

const healthSeverity: Record<HealthState, number> = { unknown: 0, healthy: 1, warning: 2, critical: 3 }

function windowTimeline(row: MonitorMatrixRow, coverage: MonitorCoverage | undefined): MonitorCardBar[] {
  const start = Date.parse(coverage?.requested_start || '')
  const end = Date.parse(coverage?.requested_end || coverage?.data_through || '')
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return []
  const width = (end - start) / MONITOR_CARD_SLOT_COUNT
  const bars: MonitorCardBar[] = Array.from({ length: MONITOR_CARD_SLOT_COUNT }, (_, index) => ({
    start: start + index * width,
    end: start + (index + 1) * width,
    state: 'unknown',
    observed: false,
  }))
  for (const bucket of row.buckets) {
    const at = Date.parse(bucket.bucket_start)
    if (at < start || at >= end || !hasMonitorSamples(bucket.metrics)) continue
    const bar = bars[Math.floor((at - start) / width)]
    if (!bar) continue
    bar.observed = true
    if (healthSeverity[bucket.health.overall] > healthSeverity[bar.state]) bar.state = bucket.health.overall
  }
  return bars
}

const MONITOR_CARD_SAMPLE_BUCKET_MS = 5 * 60 * 1000

function sampleTimeline(row: MonitorMatrixRow): MonitorCardBar[] {
  const dedicated = Array.isArray(row.sample_buckets)
  const source = dedicated ? row.sample_buckets! : row.buckets
  const sampled = source.filter((bucket) => {
    const at = Date.parse(bucket.bucket_start)
    return Number.isFinite(at) && hasMonitorSamples(bucket.metrics)
  }).slice(-MONITOR_CARD_SLOT_COUNT)
  if (!sampled.length) return []
  if (dedicated) {
    return sampled.map((bucket) => {
      const at = Date.parse(bucket.bucket_start)
      return { start: at, end: at + MONITOR_CARD_SAMPLE_BUCKET_MS, state: bucket.health.overall, observed: true }
    })
  }
  const first = Date.parse(sampled[0].bucket_start)
  const missing = MONITOR_CARD_SLOT_COUNT - sampled.length
  const bars: MonitorCardBar[] = Array.from({ length: missing }, () => ({
    start: first,
    end: first,
    state: 'unknown' as HealthState,
    observed: false,
  }))
  for (const bucket of sampled) {
    const at = Date.parse(bucket.bucket_start)
    bars.push({ start: at, end: at, state: bucket.health.overall, observed: true })
  }
  return bars
}

export function monitorCardTimeline(row: MonitorMatrixRow, coverage: MonitorCoverage | undefined, mode: MonitorCardTimelineMode = 'windows') {
  return mode === 'samples' ? sampleTimeline(row) : windowTimeline(row, coverage)
}

export function candyDisplayState(history: MonitorCandyHistory, now: number): 'correct' | 'incorrect' | 'error' | 'unknown' | 'stale' {
  const latest = history.results.at(-1)
  if (!latest) return 'unknown'
  const age = now - Date.parse(latest.checked_at)
  if (!Number.isFinite(age) || age > Math.max(180, history.interval_minutes * 60 + 120) * 1000) return 'stale'
  return latest.verdict
}
