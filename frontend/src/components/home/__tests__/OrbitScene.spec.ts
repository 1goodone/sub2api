import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// WebGL is unavailable in jsdom, so the renderer is stubbed and the test focuses on the
// component contract: it must mount, forward its settings, and stay silent about failure.
const universe = vi.hoisted(() => ({
  setPaused: vi.fn(), setLight: vi.fn(), setTitle: vi.fn(), setVisible: vi.fn(),
  refreshFonts: vi.fn(), dispose: vi.fn(), running: false, fail: false,
  onTitleVisible: undefined as ((visible: boolean) => void) | undefined
}))

vi.mock('../universe', () => ({
  Universe: class {
    running = universe.running
    constructor(_host: HTMLElement, options: { onTitleVisible?: (visible: boolean) => void }) {
      if (universe.fail) throw new Error('WebGL unavailable')
      universe.onTitleVisible = options.onTitleVisible
    }
    setPaused = universe.setPaused
    setLight = universe.setLight
    setTitle = universe.setTitle
    refreshFonts = universe.refreshFonts
    dispose = universe.dispose
  }
}))

import OrbitScene from '../OrbitScene.vue'

const chapters = [{ formation: 'core' as const, camera: {} }]
const mountScene = (props: Record<string, unknown> = {}) =>
  mount(OrbitScene, { props: { dark: true, paused: false, chapters, title: 'SUB2API', ...props } })

beforeEach(() => {
  vi.clearAllMocks()
  universe.fail = false
  universe.running = false
})

enableAutoUnmount(afterEach)
afterEach(() => { vi.restoreAllMocks() })

describe('homepage particle scene', () => {
  it('starts the universe and forwards title visibility', async () => {
    universe.running = true
    const wrapper = mountScene()
    await nextTick()
    expect(wrapper.attributes('data-animated')).toBe('true')
    universe.onTitleVisible!(true)
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('titleVisible')?.at(-1)).toEqual([true])
  })

  it('forwards pause, theme and title changes to the renderer', async () => {
    const wrapper = mountScene()
    await wrapper.setProps({ paused: true })
    expect(universe.setPaused).toHaveBeenCalledWith(true)
    await wrapper.setProps({ dark: false })
    expect(universe.setLight).toHaveBeenCalledWith(true)
    await wrapper.setProps({ title: 'CLAUDE' })
    expect(universe.setTitle).toHaveBeenCalledWith('CLAUDE')
  })

  it('falls back to the CSS starfield when WebGL cannot start', async () => {
    const warning = vi.spyOn(console, 'warn').mockImplementation(() => {})
    universe.fail = true
    const wrapper = mountScene()
    await nextTick()
    expect(wrapper.find('.orbit-starfield').exists()).toBe(true)
    expect(wrapper.find('canvas').exists()).toBe(false)
    expect(warning).toHaveBeenCalledWith('[home] particle universe unavailable', expect.any(Error))
  })

  it('disposes the renderer when the page unmounts', () => {
    const wrapper = mountScene()
    wrapper.unmount()
    expect(universe.dispose).toHaveBeenCalledOnce()
  })
})
