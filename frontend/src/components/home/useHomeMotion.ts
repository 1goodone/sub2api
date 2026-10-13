import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { homeMotion, notifyHomeMotion, resetHomeMotion } from './motionState'

interface Parallax { el: HTMLElement; speed: number; depth: number; center: number; x: number; y: number }
interface Scene { el: HTMLElement; top: number; height: number; pin: boolean; value: number }

const clamp = (v: number, lo = 0, hi = 1) => Math.min(hi, Math.max(lo, v))
const smooth = (a: number, b: number, v: number) => { const t = clamp((v - a) / (b - a)); return t * t * (3 - 2 * t) }
const damp = (rate: number, dt: number) => 1 - Math.exp(-rate * dt)

function pageTop(el: HTMLElement) {
  let top = 0
  let node: HTMLElement | null = el
  while (node) { top += node.offsetTop; node = node.offsetParent as HTMLElement | null }
  return top
}

/**
 * Drives the homepage from scroll and pointer input:
 * - `[data-chapter]` sections map scroll to `homeMotion.chapterPos` for the particle universe;
 * - `[data-pin]` sections get `--pp` (0..1 while pinned) and `[data-scene]` get `--sp` (0..1 while on screen);
 * - `[data-speed]` / `[data-depth]` elements get scroll and pointer parallax;
 * - `[data-reveal]` elements receive `.is-in` once they enter the viewport.
 */
export function useHomeMotion(root: Ref<HTMLElement | undefined>, lowMotion: Ref<boolean>, progress: Ref<HTMLElement | undefined>) {
  const activeChapter = ref(0)
  const scrolled = ref(false)
  let chapterTops: number[] = []
  let chapterHeights: number[] = []
  let parallax: Parallax[] = []
  let scenes: Scene[] = []
  let viewport = 800
  let docHeight = 1
  let lastScroll = -1
  let velocity = 0
  let px = 0
  let py = 0
  let frame = 0
  let measureFrame = 0
  let last = 0
  let dirty = true
  let reveal: IntersectionObserver | undefined
  let resizeObserver: ResizeObserver | undefined

  const measure = () => {
    measureFrame = 0
    const el = root.value
    if (!el) return
    viewport = window.innerHeight || 800
    const width = window.innerWidth || 1280
    homeMotion.narrow = width < 900 || width / viewport < 0.95
    const chapters = Array.from(el.querySelectorAll<HTMLElement>('[data-chapter]'))
    chapterTops = chapters.map(pageTop)
    chapterHeights = chapters.map(c => c.offsetHeight)
    parallax = Array.from(el.querySelectorAll<HTMLElement>('[data-speed], [data-depth]')).map(node => ({
      el: node,
      speed: parseFloat(node.dataset.speed || '0') || 0,
      depth: parseFloat(node.dataset.depth || '0') || 0,
      center: pageTop(node) + node.offsetHeight / 2,
      x: Number.NaN,
      y: Number.NaN
    }))
    scenes = Array.from(el.querySelectorAll<HTMLElement>('[data-scene], [data-pin]')).map(node => ({
      el: node, top: pageTop(node), height: node.offsetHeight, pin: node.hasAttribute('data-pin'), value: -1
    }))
    docHeight = document.documentElement.scrollHeight
    dirty = true
    wake()
  }
  const remeasure = () => { if (!measureFrame) measureFrame = requestAnimationFrame(measure) }

  const updateChapter = (scroll: number) => {
    const count = chapterTops.length
    if (!count) { homeMotion.chapterPos = 0; homeMotion.localFrac = 0; return }
    const center = scroll + viewport * 0.5
    const band = 0.42
    let index = 0
    for (let i = 1; i < count; i++) if ((center - chapterTops[i]) / viewport >= band) index = i
    const blend = index < count - 1 ? smooth(-band, band, (center - chapterTops[index + 1]) / viewport) : 0
    homeMotion.chapterPos = index + blend
    homeMotion.localFrac = clamp((center - chapterTops[index]) / Math.max(1, chapterHeights[index]))
    const active = blend > 0.5 ? index + 1 : index
    if (activeChapter.value !== active) activeChapter.value = active
  }

  const step = (now: number) => {
    frame = 0
    const dt = last ? Math.min(0.1, (now - last) / 1000) : 1 / 60
    last = now
    const scroll = window.scrollY
    const delta = lastScroll < 0 ? 0 : scroll - lastScroll
    velocity += ((dt > 0 ? delta / dt : 0) - velocity) * damp(9, dt)
    if (Math.abs(velocity) < 0.5) velocity = 0
    homeMotion.velocity = velocity
    const still = lowMotion.value
    const tx = still || !homeMotion.pointerActive ? 0 : homeMotion.pointerX
    const ty = still || !homeMotion.pointerActive ? 0 : homeMotion.pointerY
    px += (tx - px) * damp(4.2, dt)
    py += (ty - py) * damp(4.2, dt)
    const pointerMoving = Math.abs(px - tx) > 8e-4 || Math.abs(py - ty) > 8e-4
    const scrollChanged = scroll !== lastScroll || dirty
    lastScroll = scroll
    dirty = false
    if (scrollChanged) {
      if (delta) homeMotion.scrolledAt = now
      updateChapter(scroll)
      for (const scene of scenes) {
        const near = scroll + viewport * 1.5 > scene.top && scroll - viewport * 0.5 < scene.top + scene.height
        if (!near && scene.value >= 0) continue
        const value = scene.pin
          ? clamp((scroll - scene.top) / Math.max(1, scene.height - viewport))
          : clamp((scroll + viewport - scene.top) / (scene.height + viewport))
        if (Math.abs(value - scene.value) > 4e-4) {
          scene.value = value
          scene.el.style.setProperty(scene.pin ? '--pp' : '--sp', value.toFixed(4))
        }
      }
      if (progress.value) progress.value.style.transform = `scaleX(${clamp(scroll / Math.max(1, docHeight - viewport)).toFixed(4)})`
      const isScrolled = scroll > 24
      if (scrolled.value !== isScrolled) scrolled.value = isScrolled
      notifyHomeMotion()
    }
    const center = scroll + viewport * 0.5
    for (const item of parallax) {
      const offset = center - item.center
      if (Math.abs(offset) > viewport * 1.6 && !Number.isNaN(item.y)) continue
      const y = still ? 0 : offset * item.speed - py * item.depth * 9
      const x = still ? 0 : px * item.depth * 15
      if (Number.isNaN(item.x) || Math.abs(x - item.x) > 0.05 || Math.abs(y - item.y) > 0.05) {
        item.x = x
        item.y = y
        item.el.style.transform = `translate3d(${x.toFixed(2)}px, ${y.toFixed(2)}px, 0)`
      }
    }
    // Keep stepping while anything is still easing; otherwise sleep until the next input.
    if (pointerMoving || velocity !== 0 || scrollChanged) wake()
  }
  const wake = () => { if (!frame) frame = requestAnimationFrame(step) }

  const onPointer = (event: PointerEvent) => {
    if (event.pointerType !== 'mouse') return
    homeMotion.pointerX = event.clientX / window.innerWidth * 2 - 1
    homeMotion.pointerY = -(event.clientY / window.innerHeight * 2 - 1)
    homeMotion.pointerActive = !lowMotion.value
    wake()
  }
  const onLeave = () => { homeMotion.pointerActive = false; wake() }
  const onDown = (event: PointerEvent) => {
    if (lowMotion.value || event.button !== 0) return
    homeMotion.rippleX = event.clientX / window.innerWidth * 2 - 1
    homeMotion.rippleY = -(event.clientY / window.innerHeight * 2 - 1)
    homeMotion.rippleAt = performance.now()
  }

  const observeReveals = () => {
    const el = root.value
    if (!el) return
    const pending = Array.from(el.querySelectorAll('[data-reveal]:not(.is-in)'))
    if (lowMotion.value || typeof IntersectionObserver === 'undefined') {
      pending.forEach(node => node.classList.add('is-in'))
      return
    }
    reveal ??= new IntersectionObserver(entries => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        entry.target.classList.add('is-in')
        reveal?.unobserve(entry.target)
      }
    }, { threshold: 0.14, rootMargin: '0px 0px -6% 0px' })
    pending.forEach(node => reveal!.observe(node))
  }

  const scrollToChapter = (index: number) => {
    const top = index === 0 ? 0 : chapterTops[index] ?? 0
    window.scrollTo({ top, behavior: lowMotion.value ? 'auto' : 'smooth' })
  }

  watch(lowMotion, value => {
    if (value) homeMotion.pointerActive = false
    dirty = true
    observeReveals()
    wake()
  })

  onMounted(() => {
    resetHomeMotion()
    window.addEventListener('scroll', wake, { passive: true })
    window.addEventListener('resize', remeasure, { passive: true })
    window.addEventListener('pointermove', onPointer, { passive: true })
    window.addEventListener('pointerdown', onDown, { passive: true })
    window.addEventListener('blur', onLeave)
    document.documentElement.addEventListener('pointerleave', onLeave)
    if (typeof ResizeObserver !== 'undefined' && root.value) {
      resizeObserver = new ResizeObserver(remeasure)
      resizeObserver.observe(root.value)
    }
    measure()
    observeReveals()
    void document.fonts?.ready.then(remeasure).catch(() => {})
  })

  onBeforeUnmount(() => {
    cancelAnimationFrame(frame)
    cancelAnimationFrame(measureFrame)
    frame = 0
    window.removeEventListener('scroll', wake)
    window.removeEventListener('resize', remeasure)
    window.removeEventListener('pointermove', onPointer)
    window.removeEventListener('pointerdown', onDown)
    window.removeEventListener('blur', onLeave)
    document.documentElement.removeEventListener('pointerleave', onLeave)
    resizeObserver?.disconnect()
    reveal?.disconnect()
    resetHomeMotion()
  })

  return { activeChapter, scrolled, scrollToChapter, remeasure, observeReveals }
}
