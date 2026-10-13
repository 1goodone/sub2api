import type { Directive } from 'vue'

const finePointer = () => typeof window !== 'undefined' && typeof window.matchMedia === 'function' && window.matchMedia('(pointer: fine)').matches
const motionAllowed = (el: HTMLElement) => !el.closest('.motion-disabled')

type Handlers = { move: (event: PointerEvent) => void; leave: () => void }
const handlers = new WeakMap<HTMLElement, Handlers>()

function bind(el: HTMLElement, move: Handlers['move'], leave: Handlers['leave']) {
  el.addEventListener('pointermove', move)
  el.addEventListener('pointerleave', leave)
  handlers.set(el, { move, leave })
}

function unbind(el: HTMLElement) {
  const h = handlers.get(el)
  if (!h) return
  el.removeEventListener('pointermove', h.move)
  el.removeEventListener('pointerleave', h.leave)
  handlers.delete(el)
}

/** Pulls an element toward the cursor while hovered (`v-magnetic="0.28"`). */
export const vMagnetic: Directive<HTMLElement, number | undefined> = {
  mounted(el, binding) {
    if (!finePointer()) return
    const strength = binding.value ?? 0.28
    bind(el, event => {
      if (!motionAllowed(el)) return
      const rect = el.getBoundingClientRect()
      el.style.setProperty('--mx', `${((event.clientX - rect.left - rect.width / 2) * strength).toFixed(1)}px`)
      el.style.setProperty('--my', `${((event.clientY - rect.top - rect.height / 2) * strength).toFixed(1)}px`)
      el.classList.add('is-magnet')
    }, () => {
      el.style.setProperty('--mx', '0px')
      el.style.setProperty('--my', '0px')
      el.classList.remove('is-magnet')
    })
  },
  unmounted: unbind
}

/** 3D tilt plus a light spot that follows the cursor (`v-tilt="7"` degrees). */
export const vTilt: Directive<HTMLElement, number | undefined> = {
  mounted(el, binding) {
    if (!finePointer()) return
    const degrees = binding.value ?? 7
    bind(el, event => {
      if (!motionAllowed(el)) return
      const rect = el.getBoundingClientRect()
      const x = (event.clientX - rect.left) / rect.width
      const y = (event.clientY - rect.top) / rect.height
      el.style.setProperty('--rx', `${((0.5 - y) * degrees).toFixed(2)}deg`)
      el.style.setProperty('--ry', `${((x - 0.5) * degrees).toFixed(2)}deg`)
      el.style.setProperty('--hx', `${(x * 100).toFixed(1)}%`)
      el.style.setProperty('--hy', `${(y * 100).toFixed(1)}%`)
      el.classList.add('is-tilting')
    }, () => {
      el.style.setProperty('--rx', '0deg')
      el.style.setProperty('--ry', '0deg')
      el.classList.remove('is-tilting')
    })
  },
  unmounted: unbind
}
