import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DesktopDownloadView from '../DesktopDownloadView.vue'

const mocks = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: null,
    siteName: 'Sub2API',
    siteLogo: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn()
  },
  authStore: { isAuthenticated: false, isAdmin: false }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => mocks.appStore,
  useAuthStore: () => mocks.authStore
}))
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key })
}))
vi.mock('@/components/common/LocaleSwitcher.vue', () => ({
  default: defineComponent({ template: '<span data-testid="locale-switcher" />' })
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: defineComponent({ template: '<div data-testid="app-layout"><slot /></div>' })
}))
vi.mock('@/components/icons/Icon.vue', () => ({
  default: defineComponent({ template: '<span aria-hidden="true" />' })
}))

describe('DesktopDownloadView', () => {
  beforeEach(() => {
    mocks.appStore.fetchPublicSettings.mockReset()
    mocks.authStore.isAuthenticated = false
    mocks.authStore.isAdmin = false
  })

  it('links each platform directly to its latest installer', () => {
    const wrapper = mount(DesktopDownloadView, {
      global: {
        stubs: {
          RouterLink: defineComponent({
            props: { to: { type: String, required: true } },
            template: '<a :href="to"><slot /></a>'
          })
        }
      }
    })

    const downloadBaseUrl = 'https://downloads.xymm.win/xiaoyang/updates'
    expect(wrapper.get('[data-testid="desktop-latest-release"]').attributes('href')).toBe('#desktop-downloads')
    expect(wrapper.get('[data-testid="desktop-download-windows"]').attributes('href')).toBe(`${downloadBaseUrl}/xiaoyang-windows-x64-setup.exe`)
    expect(wrapper.get('[data-testid="desktop-download-mac-arm"]').attributes('href')).toBe(`${downloadBaseUrl}/xiaoyang-darwin-aarch64.dmg`)
    expect(wrapper.get('[data-testid="desktop-download-mac-intel"]').attributes('href')).toBe(`${downloadBaseUrl}/xiaoyang-darwin-x64.dmg`)
    expect(wrapper.findAll('[data-testid^="desktop-download-"]').every(link => link.attributes('target') === undefined)).toBe(true)
    expect(wrapper.find('[data-testid="desktop-all-releases"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('desktop.viewRelease')
    expect(wrapper.get('img[alt="小羊咩咩"]').attributes('src')).toBe('/illustrations/xiaoyang-desktop.png')
    expect(wrapper.text()).toContain('desktop.macArmArch')
  })

  it('keeps authenticated users inside the application layout', async () => {
    mocks.authStore.isAuthenticated = true

    const wrapper = mount(DesktopDownloadView, {
      global: {
        stubs: {
          RouterLink: defineComponent({
            props: { to: { type: String, required: true } },
            template: '<a :href="to"><slot /></a>'
          })
        }
      }
    })
    await flushPromises()

    expect(wrapper.get('[data-testid="app-layout"]').exists()).toBe(true)
    expect(wrapper.find('header').exists()).toBe(false)
    expect(wrapper.find('footer').exists()).toBe(false)
    expect(wrapper.get('[data-testid="desktop-download-windows"]').attributes('href')).toContain('.exe')
  })
})
