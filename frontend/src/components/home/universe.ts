import { buildFormation, buildSeeds, buildStars, drawWordmarkTexture, LOOKS, type FormationLook, type FormationName } from './formations'
import { PARTICLE_FRAGMENT, PARTICLE_VERTEX, TITLE_FRAGMENT, TITLE_VERTEX } from './shaders'
import { homeMotion, onHomeMotion } from './motionState'

export interface Pose {
  dist: number
  yaw: number
  pitch: number
  roll: number
  tx: number
  ty: number
  tz: number
  /** Projection-plane shift, moves the subject off-centre without rotating it. */
  sx: number
  sy: number
  fov: number
  /** Yaw swing applied across the chapter while scrolling through it. */
  drift: number
}

export interface Chapter {
  formation: FormationName
  camera: Partial<Pose>
  narrow?: Partial<Pose>
  fade?: number
  narrowFade?: number
  /** Opacity of the 3D wordmark in this chapter (hero only). */
  title?: number
}

export interface UniverseOptions {
  chapters: Chapter[]
  title: string
  light: boolean
  onTitleVisible?: (visible: boolean) => void
}

type Rgb = [number, number, number]
interface Palette { colors: Rgb[]; accent: Rgb; bright: Rgb; glow: number; additive: boolean }

const BASE_POSE: Pose = { dist: 10, yaw: 0, pitch: 0.12, roll: 0, tx: 0, ty: 0, tz: 0, sx: 0, sy: 0, fov: 0.82, drift: 0.1 }
const POSE_KEYS = Object.keys(BASE_POSE) as (keyof Pose)[]
const INTRO_POSE: Partial<Pose> = { dist: 17, pitch: 0.28, yaw: -0.35 }
const INTRO_SECONDS = 2.9
const RIPPLE_SECONDS = 1.5
const STAR_COUNT = 2400
const QUALITY = [
  { fraction: 1, dpr: 1.5, pixels: 3.4e6 },
  { fraction: 0.72, dpr: 1.25, pixels: 2.5e6 },
  { fraction: 0.5, dpr: 1, pixels: 1.7e6 },
  { fraction: 0.34, dpr: 0.85, pixels: 1.1e6 }
]

const hex = (value: string): Rgb => {
  const n = parseInt(value.slice(1), 16)
  return [(n >> 16 & 255) / 255, (n >> 8 & 255) / 255, (n & 255) / 255]
}
const PALETTES: Record<'dark' | 'light', Palette> = {
  dark: { colors: ['#70E0ED', '#2EC4DA', '#E3F5FA', '#5AAEDB'].map(hex), accent: hex('#F3C77B'), bright: hex('#F4FCFF'), glow: 1, additive: true },
  light: { colors: ['#0B7C8C', '#13A3B8', '#2F6F8F', '#1B5B7A'].map(hex), accent: hex('#C98A1C'), bright: hex('#0A5A6B'), glow: 0, additive: false }
}

const clamp = (v: number, lo = 0, hi = 1) => Math.min(hi, Math.max(lo, v))
const lerp = (a: number, b: number, t: number) => a + (b - a) * t
const damp = (rate: number, dt: number) => 1 - Math.exp(-rate * dt)
const smooth = (a: number, b: number, v: number) => { const t = clamp((v - a) / (b - a)); return t * t * (3 - 2 * t) }
const easeInOut = (t: number) => t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2

function deviceBudget() {
  const coarse = window.matchMedia?.('(pointer: coarse)').matches
  const cores = navigator.hardwareConcurrency || 4
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory || 8
  const shortSide = Math.min(screen.width || 1280, screen.height || 800)
  if (coarse || shortSide < 700 || memory <= 3 || cores <= 4) return { count: 14000, level: 1 }
  return { count: cores <= 8 ? 30000 : 42000, level: 0 }
}

function perspective(out: Float32Array, fov: number, aspect: number, near: number, far: number, sx: number, sy: number) {
  const f = 1 / Math.tan(fov / 2)
  const nf = 1 / (near - far)
  out.fill(0)
  out[0] = f / aspect
  out[5] = f
  out[8] = -sx
  out[9] = -sy
  out[10] = (far + near) * nf
  out[11] = -1
  out[14] = 2 * far * near * nf
}

function lookAt(out: Float32Array, ex: number, ey: number, ez: number, cx: number, cy: number, cz: number, roll: number) {
  let zx = ex - cx, zy = ey - cy, zz = ez - cz
  let len = Math.hypot(zx, zy, zz) || 1
  zx /= len; zy /= len; zz /= len
  const ux = Math.sin(roll), uy = Math.cos(roll)
  let xx = uy * zz, xy = -ux * zz, xz = ux * zy - uy * zx
  len = Math.hypot(xx, xy, xz) || 1
  xx /= len; xy /= len; xz /= len
  const yx = zy * xz - zz * xy, yy = zz * xx - zx * xz, yz = zx * xy - zy * xx
  out.set([xx, yx, zx, 0, xy, yy, zy, 0, xz, yz, zz, 0,
    -(xx * ex + xy * ey + xz * ez), -(yx * ex + yy * ey + yz * ez), -(zx * ex + zy * ey + zz * ez), 1])
}

function compile(gl: WebGLRenderingContext, vertex: string, fragment: string, attributes: string[]) {
  const shader = (type: number, source: string) => {
    const s = gl.createShader(type)
    if (!s) throw new Error('shader allocation failed')
    gl.shaderSource(s, source)
    gl.compileShader(s)
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS) && !gl.isContextLost()) throw new Error(gl.getShaderInfoLog(s) || 'shader compile failed')
    return s
  }
  const program = gl.createProgram()
  if (!program) throw new Error('program allocation failed')
  const vs = shader(gl.VERTEX_SHADER, vertex)
  const fs = shader(gl.FRAGMENT_SHADER, fragment)
  gl.attachShader(program, vs)
  gl.attachShader(program, fs)
  attributes.forEach((name, index) => gl.bindAttribLocation(program, index, name))
  gl.linkProgram(program)
  if (!gl.getProgramParameter(program, gl.LINK_STATUS) && !gl.isContextLost()) throw new Error(gl.getProgramInfoLog(program) || 'program link failed')
  gl.deleteShader(vs)
  gl.deleteShader(fs)
  return program
}

function uniforms(gl: WebGLRenderingContext, program: WebGLProgram, names: string[]) {
  const map: Record<string, WebGLUniformLocation | null> = {}
  names.forEach(name => { map[name] = gl.getUniformLocation(program, name) })
  return map
}

const PARTICLE_UNIFORMS = ['uProj', 'uView', 'uTime', 'uMotionA', 'uMotionB', 'uMotionC', 'uLookA', 'uLookB', 'uLookC', 'uMix', 'uIntro', 'uEnergy',
  'uPointer', 'uRipple', 'uAspect', 'uSize', 'uWarp', 'uFade', 'uPal0', 'uPal1', 'uPal2', 'uPal3', 'uAccent', 'uBright', 'uGlow']
const TITLE_UNIFORMS = ['uProj', 'uView', 'uCenter', 'uHalf', 'uTex', 'uOpacity', 'uReveal', 'uCut']

/**
 * A single full-viewport particle field. Scroll position (via `homeMotion.chapterPos`)
 * morphs the particles between per-chapter formations and flies the camera between
 * per-chapter poses; the pointer bends particles around the cursor and clicks send
 * a shockwave through them.
 */
export class Universe {
  readonly canvas: HTMLCanvasElement
  private gl: WebGLRenderingContext
  private program!: WebGLProgram
  private titleProgram!: WebGLProgram
  private u: Record<string, WebGLUniformLocation | null> = {}
  private tu: Record<string, WebGLUniformLocation | null> = {}
  private buffers = new Map<string, WebGLBuffer>()
  private titleTexture: WebGLTexture | null = null
  private titleAspect = 4
  private count: number
  private level: number
  private dpr = 1
  private width = 1
  private height = 1
  private frame = 0
  private last = 0
  private time = 7.5
  private introAge = 0
  private intro = true
  private paused = false
  private visible = true
  private lost = false
  private disposed = false
  private pos = 0
  private frac = 0
  private warp = 0
  private titleOpacity = 0
  private titleVisible = false
  private pointer = { x: 0, y: 0, force: 0 }
  private perf = { time: 0, frames: 0, age: 0 }
  private pending: FormationName[] = []
  private idle = 0
  private proj = new Float32Array(16)
  private view = new Float32Array(16)
  private poseA = { ...BASE_POSE }
  private poseB = { ...BASE_POSE }
  private pose = { ...BASE_POSE }
  private introPose = { ...BASE_POSE }
  private titlePose = { ...BASE_POSE }
  private stopListening: () => void
  private resizeObserver: ResizeObserver | null = null

  constructor(private host: HTMLElement, private options: UniverseOptions) {
    const canvas = document.createElement('canvas')
    canvas.className = 'universe-canvas'
    canvas.setAttribute('aria-hidden', 'true')
    const attributes: WebGLContextAttributes = { alpha: true, antialias: false, depth: true, stencil: false, premultipliedAlpha: true, preserveDrawingBuffer: false }
    const gl = (canvas.getContext('webgl', attributes) || canvas.getContext('experimental-webgl', attributes)) as WebGLRenderingContext | null
    if (!gl) throw new Error('WebGL unavailable')
    this.canvas = canvas
    this.gl = gl
    const budget = deviceBudget()
    this.count = budget.count
    this.level = budget.level
    this.setup()
    host.appendChild(canvas)
    canvas.addEventListener('webglcontextlost', this.onLost)
    canvas.addEventListener('webglcontextrestored', this.onRestored)
    if (typeof ResizeObserver !== 'undefined') {
      this.resizeObserver = new ResizeObserver(() => this.resize())
      this.resizeObserver.observe(host)
    }
    window.addEventListener('resize', this.resize, { passive: true })
    document.addEventListener('visibilitychange', this.sync)
    // Static frames still follow scrolling while motion is paused.
    this.stopListening = onHomeMotion(() => { if (this.paused) this.requestStill() })
    this.resize()
    this.sync()
  }

  /** Paused/reduced motion renders one static, settled frame per change. */
  setPaused(paused: boolean) {
    if (paused === this.paused) return
    this.paused = paused
    if (paused) { this.intro = false; this.pointer.force = 0; this.warp = 0 }
    this.sync()
  }

  setVisible(visible: boolean) {
    this.visible = visible
    this.sync()
  }

  setLight(light: boolean) {
    if (light === this.options.light) return
    this.options.light = light
    this.renderTitleTexture()
    this.requestStill()
  }

  setTitle(title: string) {
    if (title === this.options.title) return
    this.options.title = title
    this.deleteBuffer('wordmark')
    this.renderTitleTexture()
    this.requestStill()
  }

  /** Rebuilds font-dependent textures once web fonts finish loading. */
  refreshFonts() {
    this.deleteBuffer('wordmark')
    this.renderTitleTexture()
    this.requestStill()
  }

  dispose() {
    if (this.disposed) return
    this.disposed = true
    cancelAnimationFrame(this.frame)
    this.cancelIdle()
    this.stopListening()
    this.resizeObserver?.disconnect()
    window.removeEventListener('resize', this.resize)
    document.removeEventListener('visibilitychange', this.sync)
    this.canvas.removeEventListener('webglcontextlost', this.onLost)
    this.canvas.removeEventListener('webglcontextrestored', this.onRestored)
    const gl = this.gl
    if (!gl.isContextLost()) {
      this.buffers.forEach(buffer => gl.deleteBuffer(buffer))
      if (this.titleTexture) gl.deleteTexture(this.titleTexture)
      gl.deleteProgram(this.program)
      gl.deleteProgram(this.titleProgram)
      gl.getExtension('WEBGL_lose_context')?.loseContext()
    }
    this.buffers.clear()
    this.canvas.remove()
  }

  get running() {
    return this.frame !== 0
  }

  private setup() {
    const gl = this.gl
    this.program = compile(gl, PARTICLE_VERTEX, PARTICLE_FRAGMENT, ['aSeed', 'aA', 'aB', 'aC'])
    this.titleProgram = compile(gl, TITLE_VERTEX, TITLE_FRAGMENT, ['aCorner'])
    this.u = uniforms(gl, this.program, PARTICLE_UNIFORMS)
    this.tu = uniforms(gl, this.titleProgram, TITLE_UNIFORMS)
    this.buffers.clear()
    this.buffers.set('seeds', this.upload(buildSeeds(this.count)))
    const stars = buildStars(STAR_COUNT)
    this.buffers.set('stars', this.upload(stars.points))
    this.buffers.set('starSeeds', this.upload(stars.seeds))
    this.buffers.set('corners', this.upload(new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1])))
    this.titleTexture = null
    this.formation(this.options.chapters[0].formation)
    this.renderTitleTexture()
    this.queue(this.options.chapters.map(chapter => chapter.formation))
  }

  private upload(data: Float32Array) {
    const gl = this.gl
    const buffer = gl.createBuffer()
    if (!buffer) throw new Error('buffer allocation failed')
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW)
    return buffer
  }

  private deleteBuffer(name: string) {
    const buffer = this.buffers.get(name)
    if (!buffer) return
    if (!this.gl.isContextLost()) this.gl.deleteBuffer(buffer)
    this.buffers.delete(name)
  }

  private formation(name: FormationName) {
    let buffer = this.buffers.get(name)
    if (!buffer) {
      buffer = this.upload(buildFormation(name, this.count, this.options.title))
      this.buffers.set(name, buffer)
    }
    return buffer
  }

  /** Builds the remaining formations during idle time so the first frame stays fast. */
  private queue(names: FormationName[]) {
    for (const name of names) if (!this.buffers.has(name) && !this.pending.includes(name)) this.pending.push(name)
    if (this.idle || !this.pending.length) return
    const run = () => {
      this.idle = 0
      const name = this.pending.shift()
      if (name && !this.lost && !this.disposed) this.formation(name)
      if (this.pending.length) schedule()
    }
    const schedule = () => {
      const w = window as Window & { requestIdleCallback?: (cb: () => void, o?: { timeout: number }) => number }
      this.idle = w.requestIdleCallback ? w.requestIdleCallback(run, { timeout: 900 }) : window.setTimeout(run, 120)
    }
    schedule()
  }

  private cancelIdle() {
    if (!this.idle) return
    const w = window as Window & { cancelIdleCallback?: (id: number) => void }
    if (w.cancelIdleCallback) w.cancelIdleCallback(this.idle)
    window.clearTimeout(this.idle)
    this.idle = 0
  }

  private renderTitleTexture() {
    const gl = this.gl
    if (gl.isContextLost() || !this.options.title) return
    const canvas = drawWordmarkTexture(this.options.title, this.options.light)
    if (!canvas) return
    const texture = this.titleTexture || gl.createTexture()
    if (!texture) return
    gl.bindTexture(gl.TEXTURE_2D, texture)
    gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, true)
    gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, canvas)
    gl.pixelStorei(gl.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false)
    gl.generateMipmap(gl.TEXTURE_2D)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
    this.titleTexture = texture
    this.titleAspect = canvas.width / canvas.height
  }

  private onLost = (event: Event) => {
    event.preventDefault()
    this.lost = true
    this.cancelIdle()
    this.pending = []
    this.sync()
  }

  private onRestored = () => {
    this.lost = false
    this.setup()
    this.resize()
    this.sync()
  }

  private resize = () => {
    const width = this.host.clientWidth || window.innerWidth
    const height = this.host.clientHeight || window.innerHeight
    const quality = QUALITY[this.level]
    const dpr = Math.max(0.5, Math.min(window.devicePixelRatio || 1, quality.dpr, Math.sqrt(quality.pixels / Math.max(1, width * height))))
    const w = Math.max(1, Math.round(width * dpr))
    const h = Math.max(1, Math.round(height * dpr))
    if (this.canvas.width !== w || this.canvas.height !== h) {
      this.canvas.width = w
      this.canvas.height = h
    }
    this.width = width
    this.height = height
    this.dpr = dpr
    this.requestStill()
  }

  /** Starts or stops the animation loop from the current flags. */
  private sync = () => {
    const shouldRun = !this.paused && this.visible && !this.lost && !this.disposed && !document.hidden
    if (shouldRun && !this.frame) {
      this.last = performance.now()
      this.frame = requestAnimationFrame(this.tick)
    } else if (!shouldRun && this.frame) {
      cancelAnimationFrame(this.frame)
      this.frame = 0
    }
    if (!shouldRun) this.requestStill()
  }

  private requestStill() {
    if (this.frame || this.lost || this.disposed) return
    this.pos = clamp(homeMotion.chapterPos, 0, this.options.chapters.length - 1)
    this.frac = homeMotion.localFrac
    this.titleOpacity = this.titleAt(this.pos)
    this.render(false)
  }

  private tick = (now: number) => {
    this.frame = requestAnimationFrame(this.tick)
    const dt = Math.min(0.1, Math.max(0, (now - this.last) / 1000))
    this.last = now
    this.time += dt
    this.sample(dt)
    if (this.intro) {
      this.introAge += dt
      if (this.introAge >= INTRO_SECONDS) this.intro = false
    }
    const target = clamp(homeMotion.chapterPos, 0, this.options.chapters.length - 1)
    this.pos = lerp(this.pos, target, damp(4.6, dt))
    this.frac = lerp(this.frac, homeMotion.localFrac, damp(3.5, dt))
    const active = homeMotion.pointerActive && !homeMotion.narrow ? 1 : 0
    this.pointer.force = lerp(this.pointer.force, active, damp(2.8, dt))
    this.pointer.x = lerp(this.pointer.x, homeMotion.pointerX, damp(6, dt))
    this.pointer.y = lerp(this.pointer.y, homeMotion.pointerY, damp(6, dt))
    // Fast scrolling briefly stretches particles toward the camera.
    const scrollIdle = now - homeMotion.scrolledAt > 140
    const warp = scrollIdle ? 0 : clamp(Math.abs(homeMotion.velocity) / 7000, 0, 0.35)
    this.warp = lerp(this.warp, warp, damp(3.2, dt))
    let title = this.titleAt(this.pos)
    if (this.intro) title *= smooth(0.32, 0.78, this.introAge / INTRO_SECONDS)
    this.titleOpacity = lerp(this.titleOpacity, title, damp(7, dt))
    this.render(true)
  }

  /** Steps quality down when the device cannot hold ~42 fps. */
  private sample(dt: number) {
    this.perf.age += dt
    if (this.perf.age < 1.6) return
    this.perf.time += dt
    this.perf.frames++
    if (this.perf.time < 2.2) return
    const average = this.perf.time / this.perf.frames
    this.perf.time = 0
    this.perf.frames = 0
    if (average > 1 / 42 && this.level < QUALITY.length - 1) {
      this.level++
      this.resize()
    }
  }

  private get narrow() {
    return homeMotion.narrow || this.width / Math.max(1, this.height) < 0.95
  }

  private chapterPose(index: number, out: Pose, source?: Partial<Pose>) {
    const chapters = this.options.chapters
    const chapter = chapters[clamp(Math.round(index), 0, chapters.length - 1)]
    Object.assign(out, BASE_POSE, source ?? chapter.camera)
    if (!source && this.narrow) {
      // Portrait screens centre the subject and pull the camera back to fit it.
      out.sx = 0
      out.sy = 0
      Object.assign(out, chapter.narrow)
      const aspect = this.width / Math.max(1, this.height)
      out.dist *= clamp(Math.pow(1.3 / Math.max(aspect, 0.3), 0.56), 1, 2.4)
    }
    return out
  }

  private poseAt(pos: number, frac: number, out: Pose) {
    const index = Math.floor(pos)
    const t = easeInOut(clamp(pos - index))
    this.chapterPose(index, this.poseA)
    this.chapterPose(index + 1, this.poseB)
    for (const key of POSE_KEYS) out[key] = lerp(this.poseA[key], this.poseB[key], t)
    out.yaw += out.drift * (frac - 0.5)
    return out
  }

  private fadeAt(pos: number) {
    const chapters = this.options.chapters
    const index = Math.floor(pos)
    const value = (c: Chapter) => this.narrow && c.narrowFade !== undefined ? c.narrowFade : c.fade ?? 1
    return lerp(value(chapters[clamp(index, 0, chapters.length - 1)]), value(chapters[clamp(index + 1, 0, chapters.length - 1)]), clamp(pos - index))
  }

  private titleAt(pos: number) {
    const chapters = this.options.chapters
    const index = Math.floor(pos)
    const a = chapters[clamp(index, 0, chapters.length - 1)].title ?? 0
    const b = chapters[clamp(index + 1, 0, chapters.length - 1)].title ?? 0
    return lerp(a, b, clamp((pos - index) * 1.6))
  }

  private bindSlot(slot: number, buffer: WebGLBuffer | null | undefined) {
    const gl = this.gl
    if (!buffer) {
      gl.disableVertexAttribArray(slot)
      return
    }
    gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
    gl.enableVertexAttribArray(slot)
    gl.vertexAttribPointer(slot, 4, gl.FLOAT, false, 0, 0)
  }

  private setLook(motion: string, lookName: string, look: FormationLook) {
    const gl = this.gl
    gl.uniform4f(this.u[motion], look.mode, look.spin, look.tiltX, look.tiltZ)
    gl.uniform4f(this.u[lookName], look.size, look.alpha, look.jitter, 0)
  }

  private render(animated: boolean) {
    const gl = this.gl
    if (gl.isContextLost()) return
    const aspect = this.width / Math.max(1, this.height)
    const introT = this.intro ? clamp(this.introAge / INTRO_SECONDS) : 1
    const introMix = this.intro ? 1 - easeInOut(introT) : 0

    this.poseAt(this.pos, this.frac, this.pose)
    if (introMix > 0) {
      this.chapterPose(0, this.introPose, INTRO_POSE)
      for (const key of POSE_KEYS) this.pose[key] = lerp(this.pose[key], this.introPose[key], introMix)
    }
    const pose = this.pose
    const sway = animated ? Math.sin(this.time * 0.05) * 0.035 : 0
    const yaw = pose.yaw + sway + this.pointer.x * 0.1 * this.pointer.force
    const pitch = pose.pitch - this.pointer.y * 0.06 * this.pointer.force
    perspective(this.proj, pose.fov, aspect, 0.1, 140, pose.sx, pose.sy)
    lookAt(this.view,
      pose.tx + Math.sin(yaw) * Math.cos(pitch) * pose.dist,
      pose.ty + Math.sin(pitch) * pose.dist,
      pose.tz + Math.cos(yaw) * Math.cos(pitch) * pose.dist,
      pose.tx, pose.ty, pose.tz, pose.roll)

    gl.viewport(0, 0, this.canvas.width, this.canvas.height)
    gl.clearColor(0, 0, 0, 0)
    gl.clearDepth(1)
    gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT)
    gl.enable(gl.BLEND)
    gl.disable(gl.CULL_FACE)

    const drawTitle = this.titleOpacity > 0.01 && !!this.titleTexture
    const titleVisible = !!this.titleTexture && (drawTitle || this.titleAt(this.pos) > 0.01)
    if (titleVisible !== this.titleVisible) {
      this.titleVisible = titleVisible
      this.options.onTitleVisible?.(titleVisible)
    }
    if (drawTitle) this.drawTitle(aspect, introT)
    else gl.disable(gl.DEPTH_TEST)

    const palette = PALETTES[this.options.light ? 'light' : 'dark']
    const u = this.u
    gl.useProgram(this.program)
    if (palette.additive) gl.blendFunc(gl.ONE, gl.ONE)
    else gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
    gl.depthMask(false)
    gl.uniformMatrix4fv(u.uProj, false, this.proj)
    gl.uniformMatrix4fv(u.uView, false, this.view)
    gl.uniform1f(u.uTime, this.time)
    gl.uniform1f(u.uAspect, aspect)
    gl.uniform1f(u.uSize, 27 * this.dpr * Math.sqrt(Math.max(0.5, this.height / 900)) * (this.options.light ? 1.12 : 1))
    gl.uniform1f(u.uWarp, animated ? clamp(this.warp) : 0)
    gl.uniform1f(u.uGlow, palette.glow)
    palette.colors.forEach((color, i) => gl.uniform3f(u[`uPal${i}`], color[0], color[1], color[2]))
    gl.uniform3f(u.uAccent, ...palette.accent)
    gl.uniform3f(u.uBright, ...palette.bright)

    const rippleAge = (performance.now() - homeMotion.rippleAt) / 1000 / RIPPLE_SECONDS
    const ripple = animated && rippleAge >= 0 && rippleAge < 1 ? Math.pow(1 - rippleAge, 1.6) : 0

    // Distant stars: one slow layer that barely reacts to the pointer.
    this.bindSlot(0, this.buffers.get('starSeeds'))
    this.bindSlot(1, this.buffers.get('stars'))
    this.bindSlot(2, null)
    this.bindSlot(3, null)
    gl.uniform4f(u.uMotionA, 0, 0.004, 0.2, 0)
    gl.uniform4f(u.uLookA, 0.85, this.options.light ? 0.45 : 0.9, 0, 0)
    gl.uniform1f(u.uMix, 0)
    gl.uniform1f(u.uIntro, 0)
    gl.uniform1f(u.uEnergy, 0)
    gl.uniform1f(u.uFade, 1)
    gl.uniform3f(u.uPointer, this.pointer.x, this.pointer.y, this.pointer.force * 0.25)
    gl.uniform4f(u.uRipple, homeMotion.rippleX, homeMotion.rippleY, rippleAge, ripple * 0.3)
    gl.drawArrays(gl.POINTS, 0, STAR_COUNT)

    // The formation field: current chapter morphing into the next.
    const chapters = this.options.chapters
    const index = clamp(Math.floor(this.pos), 0, chapters.length - 1)
    const next = Math.min(chapters.length - 1, index + 1)
    const mix = next === index ? 0 : clamp(this.pos - index)
    const from = chapters[index].formation
    const to = chapters[next].formation
    this.bindSlot(0, this.buffers.get('seeds'))
    this.bindSlot(1, this.formation(from))
    this.bindSlot(2, mix > 1e-4 ? this.formation(to) : null)
    this.bindSlot(3, introMix > 1e-4 ? this.formation('dust') : null)
    this.setLook('uMotionA', 'uLookA', LOOKS[from])
    this.setLook('uMotionB', 'uLookB', LOOKS[to])
    this.setLook('uMotionC', 'uLookC', LOOKS.dust)
    gl.uniform1f(u.uMix, mix)
    gl.uniform1f(u.uIntro, introMix)
    gl.uniform1f(u.uEnergy, animated ? (this.intro ? 1.6 : 1) : 0)
    gl.uniform1f(u.uFade, this.fadeAt(this.pos) * (this.intro ? smooth(0, 0.22, introT) : 1))
    gl.uniform3f(u.uPointer, this.pointer.x, this.pointer.y, this.pointer.force)
    gl.uniform4f(u.uRipple, homeMotion.rippleX, homeMotion.rippleY, rippleAge, ripple)
    gl.drawArrays(gl.POINTS, 0, Math.floor(this.count * QUALITY[this.level].fraction))
    gl.depthMask(true)
  }

  /** Draws the hero wordmark as a glass plate inside the scene so particles orbit through it. */
  private drawTitle(aspect: number, introT: number) {
    const gl = this.gl
    this.chapterPose(0, this.titlePose)
    const narrow = this.narrow
    const halfWidth = this.titlePose.dist * Math.tan(this.titlePose.fov / 2) * aspect * (narrow ? 0.92 : 0.68)
    gl.useProgram(this.titleProgram)
    gl.uniformMatrix4fv(this.tu.uProj, false, this.proj)
    gl.uniformMatrix4fv(this.tu.uView, false, this.view)
    gl.uniform3f(this.tu.uCenter, 0, narrow ? 0.15 : 0.05, 0.25)
    gl.uniform2f(this.tu.uHalf, halfWidth, halfWidth / this.titleAspect)
    gl.uniform1f(this.tu.uOpacity, clamp(this.titleOpacity))
    gl.uniform1f(this.tu.uCut, 0.02)
    gl.uniform1f(this.tu.uReveal, this.intro ? smooth(0.3, 0.92, introT) * 1.3 : 1.3)
    gl.activeTexture(gl.TEXTURE0)
    gl.bindTexture(gl.TEXTURE_2D, this.titleTexture)
    gl.uniform1i(this.tu.uTex, 0)
    gl.bindBuffer(gl.ARRAY_BUFFER, this.buffers.get('corners')!)
    gl.enableVertexAttribArray(0)
    gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 0, 0)
    for (let slot = 1; slot < 4; slot++) gl.disableVertexAttribArray(slot)
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA)
    gl.disable(gl.DEPTH_TEST)
    gl.depthMask(false)
    gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
    if (this.titleOpacity > 0.45) {
      // Write the letters into the depth buffer so particles behind them are hidden.
      gl.enable(gl.DEPTH_TEST)
      gl.depthFunc(gl.LESS)
      gl.depthMask(true)
      gl.colorMask(false, false, false, false)
      gl.uniform1f(this.tu.uOpacity, 1)
      gl.uniform1f(this.tu.uCut, 0.62)
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4)
      gl.colorMask(true, true, true, true)
      gl.depthMask(false)
    } else {
      gl.disable(gl.DEPTH_TEST)
    }
  }
}
