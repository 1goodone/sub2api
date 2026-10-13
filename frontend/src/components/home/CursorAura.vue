<template>
  <div v-if="enabled" class="cursor-aura" :class="{ 'is-visible': visible, 'is-hover': hovering, 'is-down': pressed }" aria-hidden="true">
    <div ref="glow" class="cursor-glow" />
    <div ref="ring" class="cursor-ring" />
    <div ref="dot" class="cursor-dot" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

// A lagging ring + spotlight that trails the mouse. Native cursor stays visible, so
// nothing about pointing precision or accessibility changes.
const props = defineProps<{ disabled: boolean }>()

const enabled = ref(false)
const visible = ref(false)
const hovering = ref(false)
const pressed = ref(false)
const dot = ref<HTMLDivElement>()
const ring = ref<HTMLDivElement>()
const glow = ref<HTMLDivElement>()
const target = { x: -100, y: -100 }
const ringPos = { x: -100, y: -100 }
const glowPos = { x: -100, y: -100 }
let frame = 0
let last = 0
const INTERACTIVE = 'a, button, summary, [role="button"], input, textarea, select, label'

const place = (el: HTMLElement | undefined, x: number, y: number) => {
  if (el) el.style.transform = `translate3d(${x.toFixed(1)}px, ${y.toFixed(1)}px, 0)`
}
const step = (now: number) => {
  frame = 0
  const dt = last ? Math.min(0.05, (now - last) / 1000) : 1 / 60
  last = now
  const r = 1 - Math.exp(-dt * 18)
  const g = 1 - Math.exp(-dt * 5)
  ringPos.x += (target.x - ringPos.x) * r
  ringPos.y += (target.y - ringPos.y) * r
  glowPos.x += (target.x - glowPos.x) * g
  glowPos.y += (target.y - glowPos.y) * g
  place(dot.value, target.x, target.y)
  place(ring.value, ringPos.x, ringPos.y)
  place(glow.value, glowPos.x, glowPos.y)
  if (Math.abs(target.x - glowPos.x) + Math.abs(target.y - glowPos.y) > 0.3) frame = requestAnimationFrame(step)
  else last = 0
}
const wake = () => { if (!frame) frame = requestAnimationFrame(step) }

const onMove = (event: PointerEvent) => {
  if (event.pointerType !== 'mouse') return
  target.x = event.clientX
  target.y = event.clientY
  if (!visible.value) {
    // Jump into place instead of flying in from the corner.
    ringPos.x = glowPos.x = target.x
    ringPos.y = glowPos.y = target.y
    visible.value = true
  }
  hovering.value = !!(event.target as Element | null)?.closest?.(INTERACTIVE)
  wake()
}
const onLeave = () => { visible.value = false }
const onDown = () => { pressed.value = true }
const onUp = () => { pressed.value = false }

const attach = () => {
  window.addEventListener('pointermove', onMove, { passive: true })
  window.addEventListener('pointerdown', onDown, { passive: true })
  window.addEventListener('pointerup', onUp, { passive: true })
  document.documentElement.addEventListener('pointerleave', onLeave)
  window.addEventListener('blur', onLeave)
}
const detach = () => {
  cancelAnimationFrame(frame)
  frame = 0
  window.removeEventListener('pointermove', onMove)
  window.removeEventListener('pointerdown', onDown)
  window.removeEventListener('pointerup', onUp)
  document.documentElement.removeEventListener('pointerleave', onLeave)
  window.removeEventListener('blur', onLeave)
  visible.value = false
}

const update = () => {
  const fine = typeof window.matchMedia === 'function' && window.matchMedia('(hover: hover) and (pointer: fine)').matches
  const next = fine && !props.disabled
  if (next === enabled.value) return
  enabled.value = next
  if (next) attach()
  else detach()
}

onMounted(update)
watch(() => props.disabled, update)
onBeforeUnmount(detach)
</script>

<style scoped>
.cursor-aura { position: fixed; inset: 0; z-index: 70; pointer-events: none; opacity: 0; transition: opacity .4s ease; }
.cursor-aura.is-visible { opacity: 1; }
.cursor-aura > div { position: absolute; left: 0; top: 0; will-change: transform; }
.cursor-dot, .cursor-ring, .cursor-glow { border-radius: 50%; }
.cursor-dot::before, .cursor-ring::before, .cursor-glow::before {
  content: ''; position: absolute; border-radius: inherit;
  transition: transform .35s cubic-bezier(.16, 1, .3, 1), opacity .3s, border-color .3s, background .3s;
}
.cursor-dot::before { left: -3px; top: -3px; width: 6px; height: 6px; background: var(--home-accent); box-shadow: 0 0 12px var(--home-accent); }
.cursor-ring::before { left: -18px; top: -18px; width: 36px; height: 36px; border: 1px solid color-mix(in srgb, var(--home-accent) 55%, transparent); }
.cursor-glow::before {
  left: -260px; top: -260px; width: 520px; height: 520px;
  background: radial-gradient(closest-side, color-mix(in srgb, var(--home-accent) 11%, transparent), transparent);
}
.is-hover .cursor-ring::before { transform: scale(1.75); border-color: var(--home-accent); background: color-mix(in srgb, var(--home-accent) 8%, transparent); }
.is-hover .cursor-dot::before { transform: scale(.4); }
.is-down .cursor-ring::before { transform: scale(.8); }
.is-down.is-hover .cursor-ring::before { transform: scale(1.45); }
</style>
