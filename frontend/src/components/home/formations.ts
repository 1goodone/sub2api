// Particle formations for the homepage universe. Each formation is a Float32Array of
// `count` vec4 values: xyz position and `tone * 16 + value`, where tone picks a colour
// (palette / accent / bright) and value feeds the formation's animation mode
// (angular speed for orbits, flow speed for the tunnel, phase for waves).

export const TAU = Math.PI * 2
const GOLDEN = Math.PI * (3 - Math.sqrt(5))

export const Tone = { Palette: 0, Accent: 1, Bright: 2 } as const
export type Tone = typeof Tone[keyof typeof Tone]
export const Mode = { Still: 0, Orbit: 1, Flow: 2 } as const
export type Mode = typeof Mode[keyof typeof Mode]

export type FormationName = 'dust' | 'core' | 'sphere' | 'rings' | 'tunnel' | 'helix' | 'halo' | 'wordmark'

export interface FormationLook {
  mode: Mode
  /** Rigid rotation speed around Y (rad/s). */
  spin: number
  tiltX: number
  tiltZ: number
  size: number
  alpha: number
  /** Per-particle wobble radius. */
  jitter: number
}

export const LOOKS: Record<FormationName, FormationLook> = {
  dust: { mode: Mode.Still, spin: 0.02, tiltX: 0, tiltZ: 0, size: 0.9, alpha: 0.55, jitter: 0.04 },
  core: { mode: Mode.Orbit, spin: 0, tiltX: 0.44, tiltZ: -0.5, size: 1, alpha: 1, jitter: 0 },
  sphere: { mode: Mode.Still, spin: 0.085, tiltX: 0.28, tiltZ: 0.14, size: 1, alpha: 0.95, jitter: 0.006 },
  rings: { mode: Mode.Still, spin: 0.15, tiltX: 0.32, tiltZ: -0.22, size: 1, alpha: 1, jitter: 0 },
  tunnel: { mode: Mode.Flow, spin: 0.05, tiltX: 0, tiltZ: 0, size: 1.05, alpha: 1, jitter: 0 },
  helix: { mode: Mode.Still, spin: 0.32, tiltX: 0.1, tiltZ: 0.44, size: 1, alpha: 1, jitter: 0.004 },
  halo: { mode: Mode.Still, spin: 0, tiltX: 0, tiltZ: 0, size: 1, alpha: 1, jitter: 0.014 },
  wordmark: { mode: Mode.Still, spin: 0, tiltX: 0, tiltZ: 0, size: 0.92, alpha: 1, jitter: 0.01 }
}

type Rng = () => number

/** Small, fast, seedable PRNG so formations look identical on every visit. */
export function seeded(seed: number): Rng {
  let state = seed >>> 0
  return () => {
    state = (state + 0x6d2b79f5) >>> 0
    let t = state
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function gauss(rng: Rng) {
  let u = 0
  while (u === 0) u = rng()
  return Math.sqrt(-2 * Math.log(u)) * Math.cos(TAU * rng())
}

const pack = (tone: Tone, value: number) => tone * 16 + Math.min(15.999, Math.max(0, value))
/** Kepler-ish angular speed: inner particles orbit faster. */
const kepler = (radius: number, k = 0.62) => k / Math.pow(Math.max(radius, 0.35), 1.5)

/** Splits `total` into integer parts proportional to `weights`. */
function split(total: number, weights: number[]) {
  const sum = weights.reduce((a, b) => a + b, 0)
  const parts = weights.map(w => Math.floor(total * w / sum))
  parts[parts.length - 1] += total - parts.reduce((a, b) => a + b, 0)
  return parts
}

class Builder {
  readonly data: Float32Array
  private cursor = 0
  constructor(readonly count: number) { this.data = new Float32Array(count * 4) }
  get written() { return this.cursor }
  push(x: number, y: number, z: number, w: number) {
    if (this.cursor >= this.count) return
    this.data.set([x, y, z, w], this.cursor++ * 4)
  }
  /** Fills unused slots with copies, then shuffles so morphs pair particles randomly. */
  finish(rng: Rng) {
    const d = this.data
    for (let i = this.cursor; i < this.count; i++) {
      const from = Math.floor(rng() * Math.max(1, this.cursor)) * 4
      d.copyWithin(i * 4, from, from + 4)
    }
    for (let i = this.count - 1; i > 0; i--) {
      const j = Math.floor(rng() * (i + 1))
      for (let k = 0; k < 4; k++) {
        const tmp = d[i * 4 + k]
        d[i * 4 + k] = d[j * 4 + k]
        d[j * 4 + k] = tmp
      }
    }
    return d
  }
}

/** Evenly spaced points on a sphere shell (Fibonacci lattice). */
function shell(b: Builder, n: number, radius: number, rng: Rng, tone: () => Tone, value: number, noise = 0.02) {
  for (let i = 0; i < n; i++) {
    const y = 1 - (i + 0.5) / n * 2
    const r = Math.sqrt(Math.max(0, 1 - y * y))
    const a = i * GOLDEN
    const rr = radius + gauss(rng) * noise
    b.push(Math.cos(a) * r * rr, y * rr, Math.sin(a) * r * rr, pack(tone(), value))
  }
}

/** A small glowing body: mostly on its surface, some filling the volume. */
function body(b: Builder, n: number, cx: number, cy: number, cz: number, radius: number, rng: Rng, tone: () => Tone, value: number, surface = 0.5) {
  for (let i = 0; i < n; i++) {
    const u = rng() * 2 - 1
    const a = rng() * TAU
    const s = Math.sqrt(1 - u * u)
    const r = radius * (rng() < surface ? 0.92 + rng() * 0.08 : Math.cbrt(rng()))
    b.push(cx + s * Math.cos(a) * r, cy + u * r, cz + s * Math.sin(a) * r, pack(tone(), value))
  }
}

/** Uniform points in a spherical volume, flattened on Y. */
function cloud(b: Builder, n: number, inner: number, spread: number, flatten: number, rng: Rng, tone: () => Tone, value: (r: number) => number) {
  for (let i = 0; i < n; i++) {
    const u = rng() * 2 - 1
    const a = rng() * TAU
    const s = Math.sqrt(1 - u * u)
    const r = inner + rng() * spread
    b.push(s * Math.cos(a) * r, u * r * flatten, s * Math.sin(a) * r, pack(tone(), value(r)))
  }
}
function dust(count: number, rng: Rng) {
  const b = new Builder(count)
  for (let i = 0; i < count; i++) {
    const u = rng() * 2 - 1
    const a = rng() * TAU
    const s = Math.sqrt(1 - u * u)
    const r = 8 + Math.pow(rng(), 0.7) * 16
    b.push(s * Math.cos(a) * r, u * r * 0.7, s * Math.sin(a) * r, pack(rng() < 0.1 ? Tone.Bright : Tone.Palette, 0))
  }
  return b.finish(rng)
}

/** Hero: a glowing core with a Keplerian accretion disc, an orbit lane and its moon. */
function core(count: number, rng: Rng) {
  const b = new Builder(count)
  const [nCore, nDisc, nOuter, nLane, nMoon, nHalo] = split(count, [17, 51, 15, 8, 1.3, 7.7])
  shell(b, nCore, 1.02, rng, () => rng() < 0.26 ? Tone.Bright : Tone.Palette, 0.16, 0.018)
  for (let i = 0; i < nDisc; i++) {
    let r = rng() < 0.72 ? 2.05 + gauss(rng) * 0.26 : 1.5 + rng() * 1.6
    if (r > 2.4 && r < 2.5) r += 0.12 // a thin gap reads as structure
    r = Math.min(3.15, Math.max(1.42, r))
    const a = rng() * TAU
    b.push(Math.cos(a) * r, gauss(rng) * 0.022 * r, Math.sin(a) * r, pack(rng() < 0.13 ? Tone.Bright : Tone.Palette, kepler(r)))
  }
  for (let i = 0; i < nOuter; i++) {
    const r = 3.3 + Math.pow(rng(), 1.6) * 3.6
    const a = rng() * TAU
    b.push(Math.cos(a) * r, gauss(rng) * 0.09, Math.sin(a) * r, pack(Tone.Palette, kepler(r)))
  }
  const lane = 3.4
  const laneSpeed = kepler(lane)
  for (let i = 0; i < nLane; i++) {
    const a = rng() * TAU
    const r = lane + gauss(rng) * 0.014
    b.push(Math.cos(a) * r, gauss(rng) * 0.01, Math.sin(a) * r, pack(Tone.Bright, laneSpeed))
  }
  body(b, nMoon, Math.cos(0.85) * lane, 0, Math.sin(0.85) * lane, 0.13, rng, () => Tone.Accent, laneSpeed, 0.55)
  cloud(b, nHalo, 2.5, 6.5, 0.55, rng, () => rng() < 0.08 ? Tone.Accent : Tone.Palette, r => kepler(r, 0.3))
  return b.finish(rng)
}

/** Unified access: one sphere with model "planets" linked to it by arcs. */
function sphere(count: number, rng: Rng) {
  const b = new Builder(count)
  const [nShell, nPlanets, nLinks, nHalo] = split(count, [44, 30, 19, 7])
  shell(b, nShell, 2, rng, () => rng() < 0.16 ? Tone.Bright : Tone.Palette, 0, 0.03)
  const planets: [number, number, number, number][] = []
  for (let i = 0; i < 7; i++) {
    const a = i / 7 * TAU + 0.35
    const r = 3.55 + (i % 2) * 0.35
    planets.push([Math.cos(a) * r, Math.sin(a * 2) * 0.75 + (i % 3 - 1) * 0.4, Math.sin(a) * r, 0.2 + (i * 7 % 5) * 0.045])
  }
  const planetCounts = split(nPlanets, planets.map(p => p[3]))
  planets.forEach((p, i) => body(b, planetCounts[i], p[0], p[1], p[2], p[3], rng, () => i === 2 ? Tone.Accent : rng() < 0.3 ? Tone.Bright : Tone.Palette, 0, 0.55))
  const linkCounts = split(nLinks, planets.map(() => 1))
  planets.forEach((p, i) => {
    const len = Math.hypot(p[0], p[1], p[2])
    const s = [p[0] / len * 2, p[1] / len * 2, p[2] / len * 2]
    const c = [(s[0] + p[0]) * 0.59, (s[1] + p[1]) * 0.5 + 0.55, (s[2] + p[2]) * 0.59]
    for (let k = 0; k < linkCounts[i]; k++) {
      const t = rng()
      const w0 = (1 - t) * (1 - t)
      const w1 = 2 * (1 - t) * t
      const w2 = t * t
      b.push(
        w0 * s[0] + w1 * c[0] + w2 * p[0] + gauss(rng) * 0.012,
        w0 * s[1] + w1 * c[1] + w2 * p[1] + gauss(rng) * 0.012,
        w0 * s[2] + w1 * c[2] + w2 * p[2] + gauss(rng) * 0.012,
        pack(rng() < 0.32 ? Tone.Bright : Tone.Palette, 0)
      )
    }
  })
  cloud(b, nHalo, 4.6, 4.5, 0.7, rng, () => Tone.Palette, () => 0)
  return b.finish(rng)
}
/** Capabilities: an atom-like gyroscope of tilted rings carrying bright nodes. */
function rings(count: number, rng: Rng) {
  const b = new Builder(count)
  const [nNucleus, nRings, nNodes, nBelt] = split(count, [13, 73, 5, 9])
  for (let i = 0; i < nNucleus; i++) {
    const r = Math.abs(gauss(rng)) * 0.3
    const u = rng() * 2 - 1
    const a = rng() * TAU
    const s = Math.sqrt(1 - u * u)
    b.push(s * Math.cos(a) * r, u * r, s * Math.sin(a) * r, pack(rng() < 0.45 ? Tone.Bright : Tone.Palette, 0))
  }
  const ringCount = 6
  const place = (ring: number, angle: number, radius: number): [number, number, number] => {
    const yaw = ring / ringCount * Math.PI
    const tilt = 1.12 + (ring % 2) * 0.3
    const x = Math.cos(angle) * radius
    const y = Math.sin(angle) * radius
    const ty = y * Math.sin(tilt)
    const tz = y * Math.cos(tilt)
    return [x * Math.cos(yaw) + tz * Math.sin(yaw), ty, -x * Math.sin(yaw) + tz * Math.cos(yaw)]
  }
  const ringCounts = split(nRings, Array.from({ length: ringCount }, () => 1))
  for (let ring = 0; ring < ringCount; ring++) {
    const radius = 2.45 + (ring % 3) * 0.08
    for (let k = 0; k < ringCounts[ring]; k++) {
      const [x, y, z] = place(ring, rng() * TAU, radius)
      const n = gauss(rng) * 0.012
      b.push(x + n, y + n, z + n, pack(rng() < 0.12 ? Tone.Bright : Tone.Palette, 0))
    }
  }
  const nodeCounts = split(nNodes, Array.from({ length: ringCount }, () => 1))
  for (let ring = 0; ring < ringCount; ring++) {
    const [x, y, z] = place(ring, ring * 1.7 + 0.4, 2.48)
    body(b, nodeCounts[ring], x, y, z, 0.09, rng, () => ring % 2 ? Tone.Accent : Tone.Bright, 0, 0.4)
  }
  for (let i = 0; i < nBelt; i++) {
    const a = rng() * TAU
    const r = 4.1 + gauss(rng) * 0.05
    b.push(Math.cos(a) * r, gauss(rng) * 0.02, Math.sin(a) * r, pack(Tone.Palette, 0))
  }
  return b.finish(rng)
}

/** Real-time streaming: a warp tunnel rushing toward the camera. */
function tunnel(count: number, rng: Rng) {
  const b = new Builder(count)
  for (let i = 0; i < count; i++) {
    const a = rng() * TAU
    const r = 1.5 + Math.pow(rng(), 0.75) * 5.2
    const tone = rng() < 0.16 ? Tone.Bright : rng() < 0.03 ? Tone.Accent : Tone.Palette
    b.push(Math.cos(a) * r, Math.sin(a) * r, -34 + rng() * 42, pack(tone, 5 + rng() * 9))
  }
  return b.finish(rng)
}

/** Getting started: a DNA-like double helix with rungs. */
function helix(count: number, rng: Rng) {
  const b = new Builder(count)
  const [nStrands, nRungs, nHalo] = split(count, [56, 24, 20])
  const height = 8.6
  const turns = 2.6
  const radius = 1.35
  for (let i = 0; i < nStrands; i++) {
    const t = rng()
    const a = t * turns * TAU + (i % 2) * Math.PI
    b.push(Math.cos(a) * radius + gauss(rng) * 0.05, -height / 2 + t * height + gauss(rng) * 0.05, Math.sin(a) * radius + gauss(rng) * 0.05,
      pack(rng() < 0.16 ? Tone.Bright : Tone.Palette, 0))
  }
  const rungs = 34
  const rungCounts = split(nRungs, Array.from({ length: rungs }, () => 1))
  for (let r = 0; r < rungs; r++) {
    const t = (r + 0.5) / rungs
    const a = t * turns * TAU
    const x = Math.cos(a) * radius
    const z = Math.sin(a) * radius
    const y = -height / 2 + t * height
    for (let k = 0; k < rungCounts[r]; k++) {
      const s = 1 - 2 * rng()
      b.push(x * s, y + gauss(rng) * 0.012, z * s, pack(r % 9 === 4 ? Tone.Accent : rng() < 0.42 ? Tone.Bright : Tone.Palette, 0))
    }
  }
  for (let i = 0; i < nHalo; i++) {
    const a = rng() * TAU
    const r = 2.2 + Math.pow(rng(), 0.8) * 3.6
    b.push(Math.cos(a) * r, -5.5 + rng() * 11, Math.sin(a) * r, pack(Tone.Palette, 0))
  }
  return b.finish(rng)
}

/** FAQ: a calm halo ring with a tilted ellipse, framing text instead of competing with it. */
function halo(count: number, rng: Rng) {
  const b = new Builder(count)
  const [nRing, nEllipse, nSpark, nField] = split(count, [34, 36, 5, 25])
  for (let i = 0; i < nRing; i++) {
    const a = rng() * TAU
    const r = 1.5 + (rng() - 0.5) * 0.58
    b.push(Math.cos(a) * r, Math.sin(a) * r, gauss(rng) * 0.08, pack(rng() < 0.28 ? Tone.Bright : Tone.Palette, 0))
  }
  const rot = -35 * Math.PI / 180
  for (let i = 0; i < nEllipse; i++) {
    const a = rng() * TAU
    const x = Math.cos(a) * 3 + gauss(rng) * 0.05
    const y = Math.sin(a) * 1.17 + gauss(rng) * 0.05
    b.push(x * Math.cos(rot) - y * Math.sin(rot), x * Math.sin(rot) + y * Math.cos(rot), Math.sin(a) * 0.45 + gauss(rng) * 0.04,
      pack(rng() < 0.1 ? Tone.Bright : Tone.Palette, 0))
  }
  body(b, nSpark, 2.33, 1.58, 0.35, 0.42, rng, () => Tone.Accent, 0, 0.2)
  for (let i = 0; i < nField; i++) {
    const a = rng() * TAU
    const r = 3.6 + Math.pow(rng(), 0.8) * 5
    b.push(Math.cos(a) * r, Math.sin(a) * r * 0.62, -1 - rng() * 3, pack(rng() < 0.05 ? Tone.Accent : Tone.Palette, 0))
  }
  return b.finish(rng)
}
const FONT_FAMILY = '"Manrope Variable", "Manrope", "PingFang SC", "Microsoft YaHei", "Segoe UI", Arial, sans-serif'
let wordmarkCache: { key: string; points: [number, number][] } | null = null

/** Samples the opaque pixels of `text` rendered to a 2D canvas. */
function textPoints(text: string): [number, number][] {
  if (typeof document === 'undefined') return []
  const fontReady = !!document.fonts?.check?.('800 100px "Manrope Variable"')
  const key = `${text}|${fontReady}`
  if (wordmarkCache?.key === key) return wordmarkCache.points
  const canvas = document.createElement('canvas')
  const width = 1200
  const height = 300
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d', { willReadFrequently: true })
  if (!ctx) return []
  let size = 250
  ctx.font = `800 ${size}px ${FONT_FAMILY}`
  const measured = ctx.measureText(text).width
  if (measured > width * 0.96) {
    size = Math.floor(size * width * 0.96 / measured)
    ctx.font = `800 ${size}px ${FONT_FAMILY}`
  }
  ctx.fillStyle = '#fff'
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  ctx.fillText(text, width / 2, height / 2 + size * 0.04)
  const pixels = ctx.getImageData(0, 0, width, height).data
  const points: [number, number][] = []
  for (let y = 0; y < height; y += 2) {
    for (let x = 0; x < width; x += 2) {
      if (pixels[(y * width + x) * 4 + 3] > 150) points.push([(x - width / 2) / width, -(y - height / 2) / width])
    }
  }
  wordmarkCache = { key, points }
  return points
}

/** Finale: the site name drawn in particles, wrapped by an orbit. */
function wordmark(count: number, rng: Rng, text: string) {
  const b = new Builder(count)
  const [nText, nOrbit, nMoon, nField] = split(count, [78, 13, 2, 7])
  const points = textPoints(text)
  const scale = 10.4
  if (points.length) {
    for (let i = 0; i < nText; i++) {
      const p = points[Math.floor(rng() * points.length)]
      b.push(p[0] * scale + (rng() - 0.5) * 0.02, p[1] * scale + (rng() - 0.5) * 0.02, gauss(rng) * 0.07, pack(rng() < 0.18 ? Tone.Bright : Tone.Palette, 0))
    }
  } else {
    body(b, nText, 0, 0, 0, 1.6, rng, () => Tone.Palette, 0, 0.9)
  }
  const rot = -0.09
  for (let i = 0; i < nOrbit; i++) {
    const a = rng() * TAU
    const x = Math.cos(a) * 6.5
    const y = Math.sin(a) * 1.05
    b.push(x * Math.cos(rot) - y * Math.sin(rot), x * Math.sin(rot) + y * Math.cos(rot) - 0.05, Math.sin(a) * 1.5, pack(rng() < 0.2 ? Tone.Bright : Tone.Palette, 0))
  }
  body(b, nMoon, Math.cos(-0.5) * 6.5, Math.sin(-0.5) * 1.05 - 0.6, 0.6, 0.12, rng, () => Tone.Accent, 0, 0.3)
  for (let i = 0; i < nField; i++) b.push((rng() - 0.5) * 18, (rng() - 0.5) * 8, -2 - rng() * 4, pack(Tone.Palette, 0))
  return b.finish(rng)
}

const SEEDS: Record<FormationName, number> = { dust: 11, core: 23, sphere: 37, rings: 41, tunnel: 53, helix: 113, halo: 71, wordmark: 83 }

export function buildFormation(name: FormationName, count: number, text = 'SUB2API') {
  const rng = seeded(SEEDS[name] * 7919 + count)
  switch (name) {
    case 'dust': return dust(count, rng)
    case 'core': return core(count, rng)
    case 'sphere': return sphere(count, rng)
    case 'rings': return rings(count, rng)
    case 'tunnel': return tunnel(count, rng)
    case 'helix': return helix(count, rng)
    case 'halo': return halo(count, rng)
    case 'wordmark': return wordmark(count, rng, text)
  }
}

/** Per-particle random seeds shared by every formation (stable pairing during morphs). */
export function buildSeeds(count: number) {
  const rng = seeded(20261007)
  const seeds = new Float32Array(count * 4)
  for (let i = 0; i < seeds.length; i++) seeds[i] = rng()
  return seeds
}

/** A distant star shell drawn behind every formation. */
export function buildStars(count: number) {
  const rng = seeded(424242)
  const b = new Builder(count)
  for (let i = 0; i < count; i++) {
    const u = rng() * 2 - 1
    const a = rng() * TAU
    const s = Math.sqrt(1 - u * u)
    const r = 26 + rng() * 18
    b.push(s * Math.cos(a) * r, u * r, s * Math.sin(a) * r, pack(rng() < 0.04 ? Tone.Accent : rng() < 0.3 ? Tone.Bright : Tone.Palette, 0))
  }
  const seeds = new Float32Array(count * 4)
  for (let i = 0; i < seeds.length; i++) seeds[i] = rng()
  return { points: b.finish(rng), seeds }
}

/** Renders the hero wordmark as a glassy texture (fill gradient, glowing rim, top sheen). */
export function drawWordmarkTexture(text: string, light: boolean) {
  const canvas = document.createElement('canvas')
  canvas.width = 2048
  canvas.height = 512
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  let size = 380
  ctx.font = `800 ${size}px ${FONT_FAMILY}`
  size = Math.floor(Math.min(470, size * 1880 / Math.max(1, ctx.measureText(text).width)))
  ctx.font = `800 ${size}px ${FONT_FAMILY}`
  ctx.textAlign = 'center'
  ctx.textBaseline = 'middle'
  const cx = canvas.width / 2
  const cy = canvas.height / 2 + size * 0.035
  const top = cy - size * 0.38
  const bottom = cy + size * 0.38
  const fill = ctx.createLinearGradient(0, top, 0, bottom)
  if (light) {
    fill.addColorStop(0, '#276276')
    fill.addColorStop(0.5, '#164759')
    fill.addColorStop(1, '#0a2e3e')
  } else {
    fill.addColorStop(0, '#2c5470')
    fill.addColorStop(0.42, '#10283b')
    fill.addColorStop(1, '#081624')
  }
  ctx.lineJoin = 'round'
  ctx.shadowColor = light ? 'rgba(7, 120, 140, 0.35)' : 'rgba(112, 224, 237, 0.42)'
  ctx.shadowBlur = 26
  ctx.lineWidth = 5
  ctx.strokeStyle = light ? 'rgba(10, 128, 148, 0.9)' : 'rgba(150, 236, 245, 0.95)'
  ctx.strokeText(text, cx, cy)
  ctx.shadowBlur = 0
  ctx.fillStyle = fill
  ctx.fillText(text, cx, cy)
  const sheen = ctx.createLinearGradient(0, top, 0, cy)
  sheen.addColorStop(0, light ? 'rgba(255, 255, 255, 0.25)' : 'rgba(190, 245, 250, 0.38)')
  sheen.addColorStop(1, 'rgba(255, 255, 255, 0)')
  ctx.globalCompositeOperation = 'source-atop'
  ctx.fillStyle = sheen
  ctx.fillRect(0, top - 20, canvas.width, cy - top + 20)
  ctx.globalCompositeOperation = 'source-over'
  return canvas
}
