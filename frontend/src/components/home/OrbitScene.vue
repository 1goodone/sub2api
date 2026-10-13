<template>
  <div ref="host" class="orbit-universe" :class="{ 'is-paused': paused }" aria-hidden="true" :data-animated="running">
    <div class="orbit-nebula orbit-nebula-a" />
    <div class="orbit-nebula orbit-nebula-b" />
    <div v-if="!supported" class="orbit-starfield" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Universe, type Chapter } from './universe'

const props = withDefaults(defineProps<{
  dark: boolean
  paused: boolean
  chapters?: Chapter[]
  title?: string
}>(), {
  chapters: () => [{ formation: 'core', camera: {} }],
  title: ''
})
const emit = defineEmits<{ titleVisible: [visible: boolean] }>()

const host = ref<HTMLDivElement>()
const supported = ref(true)
const running = ref(false)
let universe: Universe | null = null

const syncRunning = () => { running.value = !!universe?.running }

onMounted(() => {
  if (!host.value) return
  try {
    universe = new Universe(host.value, {
      chapters: props.chapters,
      title: props.title,
      light: !props.dark,
      onTitleVisible: visible => emit('titleVisible', visible)
    })
  } catch (error) {
    // The page stays fully usable without WebGL; a CSS starfield takes over.
    console.warn('[home] particle universe unavailable', error)
    supported.value = false
    emit('titleVisible', false)
    return
  }
  universe.setPaused(props.paused)
  syncRunning()
  void document.fonts?.ready.then(() => universe?.refreshFonts()).catch(() => {})
})

watch(() => props.paused, paused => { universe?.setPaused(paused); syncRunning() })
watch(() => props.dark, dark => universe?.setLight(!dark))
watch(() => props.title, title => universe?.setTitle(title))

onBeforeUnmount(() => {
  universe?.dispose()
  universe = null
})
</script>

<style scoped>
.orbit-universe {
  position: fixed;
  inset: 0;
  z-index: 0;
  height: 100lvh;
  overflow: hidden;
  pointer-events: none;
  background: var(--universe-bg);
}
.orbit-universe :deep(.universe-canvas) {
  position: absolute;
  inset: 0;
  display: block;
  width: 100%;
  height: 100%;
}
.orbit-nebula {
  position: absolute;
  inset: -20%;
  pointer-events: none;
  animation: orbit-drift 80s cubic-bezier(.65, 0, .35, 1) infinite alternate;
}
.orbit-nebula-a { background: var(--nebula-a); }
.orbit-nebula-b { background: var(--nebula-b); animation-duration: 110s; animation-direction: alternate-reverse; }
.is-paused .orbit-nebula { animation-play-state: paused; }
@keyframes orbit-drift {
  from { transform: translate3d(-2%, -1.5%, 0) scale(1); }
  to { transform: translate3d(2.5%, 2%, 0) scale(1.06); }
}
.orbit-starfield {
  position: absolute;
  inset: 0;
  opacity: .7;
  background-size: 420px 420px;
  background-image:
    radial-gradient(1px 1px at 12% 18%, var(--star-a), transparent),
    radial-gradient(1px 1px at 72% 12%, var(--star-b), transparent),
    radial-gradient(1.5px 1.5px at 42% 62%, var(--star-c), transparent),
    radial-gradient(1px 1px at 86% 70%, var(--star-a), transparent),
    radial-gradient(1px 1px at 24% 86%, var(--star-d), transparent),
    radial-gradient(1px 1px at 58% 34%, var(--star-b), transparent);
}
@media (prefers-reduced-motion: reduce) {
  .orbit-nebula { animation: none; }
}
</style>
