import type { MonitorCoverage, MonitorMatrixBucket } from '@/api/channelMonitorV2'

export interface V3TimelineSlot {
  start: string
  bucket?: MonitorMatrixBucket
}

function parseTime(value?: string | null): number {
  if (!value) return Number.NaN
  return Date.parse(value)
}

function toSlotIso(ms: number): string {
  return new Date(ms).toISOString()
}

function bucketStepMs(coverage?: Pick<MonitorCoverage, 'bucket_seconds'> | null): number {
  return Math.max(60, coverage?.bucket_seconds ?? 300) * 1000
}

/** Clock-aligned slot starts for the selected range. Always `length` entries when coverage has a start. */
export function listV3TimelineSlotStarts(
  coverage: Pick<MonitorCoverage, 'requested_start' | 'bucket_seconds'> | null | undefined,
  length: number,
): string[] {
  const requestedStart = parseTime(coverage?.requested_start)
  if (!Number.isFinite(requestedStart) || length <= 0) return []
  const step = bucketStepMs(coverage)
  const origin = Math.floor(requestedStart / step) * step
  return Array.from({ length }, (_, index) => toSlotIso(origin + index * step))
}

export function alignV3TimelineSlots(
  buckets: MonitorMatrixBucket[] | undefined,
  coverage: Pick<MonitorCoverage, 'requested_start' | 'bucket_seconds'> | null | undefined,
  length: number,
): V3TimelineSlot[] {
  const starts = listV3TimelineSlotStarts(coverage, length)
  if (starts.length === 0) {
    const real = [...(buckets ?? [])]
      .filter(bucket => Boolean(bucket.bucket_start))
      .sort((a, b) => Date.parse(a.bucket_start) - Date.parse(b.bucket_start))
      .slice(-Math.max(0, length))
    const pad = Math.max(0, length - real.length)
    return [
      ...Array.from({ length: pad }, () => ({ start: '' })),
      ...real.map(bucket => ({ start: bucket.bucket_start, bucket })),
    ]
  }

  const step = bucketStepMs(coverage)
  const byStart = new Map<string, MonitorMatrixBucket>()
  for (const bucket of buckets ?? []) {
    const ts = Date.parse(bucket.bucket_start)
    if (!Number.isFinite(ts)) continue
    byStart.set(toSlotIso(Math.floor(ts / step) * step), bucket)
  }
  return starts.map(start => ({ start, bucket: byStart.get(start) }))
}
