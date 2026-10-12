// Shared, non-reactive motion state. The page scroll/pointer controller writes it and the
// particle universe reads it every frame, so neither side pays for Vue reactivity.
export interface HomeMotion {
  /** Fractional chapter index: 1.5 means halfway between chapter 1 and 2. */
  chapterPos: number
  /** Progress through the current chapter, 0..1. */
  localFrac: number
  /** Smoothed scroll velocity in px/s. */
  velocity: number
  scrolledAt: number
  /** Pointer in normalized device coordinates (y up). */
  pointerX: number
  pointerY: number
  pointerActive: boolean
  rippleX: number
  rippleY: number
  rippleAt: number
  narrow: boolean
}

const initial = (): HomeMotion => ({
  chapterPos: 0,
  localFrac: 0,
  velocity: 0,
  scrolledAt: 0,
  pointerX: 0,
  pointerY: 0,
  pointerActive: false,
  rippleX: 0,
  rippleY: 0,
  rippleAt: -1e9,
  narrow: false
})

export const homeMotion: HomeMotion = initial()

const listeners = new Set<() => void>()

/** Subscribe to scroll/layout changes (not pointer moves). */
export function onHomeMotion(listener: () => void) {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

export function notifyHomeMotion() {
  listeners.forEach(listener => listener())
}

export function resetHomeMotion() {
  Object.assign(homeMotion, initial())
}
