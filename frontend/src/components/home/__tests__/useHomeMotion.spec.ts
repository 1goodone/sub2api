import { afterEach, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { useHomeMotion } from '../useHomeMotion'

afterEach(() => vi.unstubAllGlobals())

it('initializes parallax, follows mouse input, and resets when motion is paused', async () => {
  let id = 0
  const frames = new Map<number, FrameRequestCallback>()
  vi.stubGlobal('requestAnimationFrame', (callback: FrameRequestCallback) => {
    frames.set(++id, callback)
    return id
  })
  vi.stubGlobal('cancelAnimationFrame', (key: number) => frames.delete(key))
  const frame = (time: number) => {
    const pending = [...frames.values()]
    frames.clear()
    pending.forEach(callback => callback(time))
  }
  const paused = ref(false)
  const wrapper = mount(defineComponent({
    setup() {
      const root = ref<HTMLElement>()
      useHomeMotion(root, paused, ref())
      return { root }
    },
    template: '<main ref="root"><div data-depth="2" /></main>'
  }))
  try {
    const target = wrapper.get('[data-depth]').element as HTMLElement
    frame(16)
    expect(target.style.transform).toBe('translate3d(0.00px, 0.00px, 0)')
    const move = new MouseEvent('pointermove', { clientX: window.innerWidth, clientY: 0 })
    Object.defineProperty(move, 'pointerType', { value: 'mouse' })
    window.dispatchEvent(move)
    frame(32)
    expect(target.style.transform).not.toBe('translate3d(0.00px, 0.00px, 0)')
    paused.value = true
    await nextTick()
    frame(48)
    expect(target.style.transform).toBe('translate3d(0.00px, 0.00px, 0)')
  } finally {
    wrapper.unmount()
  }
  expect(frames.size).toBe(0)
})
