import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, mount, RouterLinkStub } from '@vue/test-utils'

import HomeView from '../HomeView.vue'

enableAutoUnmount(afterEach)

const { appStore, authStore } = vi.hoisted(() => ({
  appStore: {
    cachedPublicSettings: {} as Record<string, unknown>,
    siteName: 'Fallback site',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings: vi.fn(),
  },
  authStore: {
    isAuthenticated: false,
    isAdmin: false,
    user: null as { email?: string } | null,
    checkAuth: vi.fn(),
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function mountHome(settings: Record<string, unknown> = {}) {
  appStore.cachedPublicSettings = {
    site_name: 'Test site',
    site_subtitle: 'Test subtitle',
    ...settings,
  }

  return mount(HomeView, {
    global: {
      stubs: {
        RouterLink: RouterLinkStub,
        LocaleSwitcher: { template: '<div data-testid="locale-switcher" />' },
        Icon: { template: '<span data-testid="icon" />' },
        OrbitScene: { template: '<div data-testid="orbit-scene" />', props: ['dark', 'paused'] },
      },
    },
  })
}

function compactDestination(wrapper: ReturnType<typeof mountHome>) {
  return wrapper.get('[data-testid="compact-home"] main').findComponent(RouterLinkStub).props('to')
}

describe('HomeView compact mode', () => {
  beforeEach(() => {
    authStore.isAuthenticated = false
    authStore.isAdmin = false
    authStore.user = null
    authStore.checkAuth.mockClear()
    appStore.fetchPublicSettings.mockClear()
    localStorage.clear()
    vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
  })

  it('renders custom HTML ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: '<section id="custom-home">Custom home</section>',
    })

    expect(wrapper.get('#custom-home').text()).toBe('Custom home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('renders custom URL content ahead of compact mode', () => {
    const wrapper = mountHome({
      compact_home_enabled: true,
      home_content: ' https://example.com/home ',
    })

    expect(wrapper.get('iframe').attributes('src')).toBe('https://example.com/home')
    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
  })

  it('treats whitespace-only custom content as empty and selects compact mode', () => {
    const wrapper = mountHome({ compact_home_enabled: true, home_content: ' \n\t ' })

    expect(wrapper.get('[data-testid="compact-home"]').text()).toContain('Test site')
  })

  it.each([undefined, false])('selects the default home when compact mode is %s', (enabled) => {
    const settings = enabled === undefined ? {} : { compact_home_enabled: enabled }
    const wrapper = mountHome(settings)

    expect(wrapper.find('[data-testid="compact-home"]').exists()).toBe(false)
    expect(wrapper.find('.terminal-container').exists()).toBe(true)
  })

  it('links unauthenticated visitors to login', () => {
    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/login')
  })

  it('links authenticated users to their dashboard', () => {
    authStore.isAuthenticated = true

    expect(compactDestination(mountHome({ compact_home_enabled: true }))).toBe('/dashboard')
  })

  it('links administrators to the admin dashboard', () => {
    authStore.isAuthenticated = true
    authStore.isAdmin = true

    const wrapper = mountHome({ compact_home_enabled: true })
    expect(compactDestination(wrapper)).toBe('/admin/dashboard')
    expect(authStore.checkAuth).toHaveBeenCalledOnce()
    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
  })

  it.each([true, false])('does not expose the removed model plaza route: %s', (compact) => {
    const wrapper = mountHome({ compact_home_enabled: compact, model_plaza_enabled: true })
    expect(wrapper.findAllComponents(RouterLinkStub).some(link => link.props('to') === '/model-plaza')).toBe(false)
  })

  it('renders replacement connection paths in the default home', async () => {
    const wrapper = mountHome()

    expect(wrapper.get('.integration-paths').findAll('li')).toHaveLength(4)
    expect(wrapper.get('.home-nav a[href="#clients"]').text()).toBe('home.orbit.ecosystem')
    await wrapper.get('.home-menu-btn').trigger('click')
    expect(wrapper.get('.home-menu a[href="#clients"]').text()).toBe('home.orbit.ecosystem')
  })

  it.each([true, false])('respects registration availability in the default home: %s', (enabled) => {
    const wrapper = mountHome({ registration_enabled: enabled })
    expect(wrapper.get('.hero-links').findComponent(RouterLinkStub).props('to')).toBe(enabled ? '/register' : '/login')
  })

  it.each([false, true])('sends signed-in visitors to the correct workspace (admin: %s)', (admin) => {
    authStore.isAuthenticated = true
    authStore.isAdmin = admin
    const wrapper = mountHome({ registration_enabled: true })
    expect(wrapper.get('.hero-links').findComponent(RouterLinkStub).props('to')).toBe(admin ? '/admin/dashboard' : '/dashboard')
  })

  it('lets visitors pause and resume the decorative animation', async () => {
    const wrapper = mountHome()
    const scene = wrapper.getComponent('[data-testid="orbit-scene"]')
    expect(scene.props('paused')).toBe(false)
    await wrapper.get('.orbit-motion').trigger('click')
    expect(scene.props('paused')).toBe(true)
    expect(wrapper.get('.orbit-motion').attributes('aria-pressed')).toBe('true')
    await wrapper.get('.orbit-motion').trigger('click')
    expect(scene.props('paused')).toBe(false)
  })

  it('respects reduced motion before the scene starts', () => {
    const mediaQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
    vi.mocked(window.matchMedia).mockImplementation((query) => ({
      ...mediaQuery, media: query, matches: query.includes('prefers-reduced-motion'),
    }))
    const wrapper = mountHome()
    expect(wrapper.getComponent('[data-testid="orbit-scene"]').props('paused')).toBe(true)
    expect(wrapper.find('.orbit-motion').exists()).toBe(false)
  })

  it.each(['https://gateway.example.test', 'https://gateway.example.test/v1/'])('uses the configured API endpoint: %s', (apiBaseUrl) => {
    const wrapper = mountHome({ api_base_url: apiBaseUrl })
    expect(wrapper.get('.endpoint-url code').text()).toBe('https://gateway.example.test/v1')
  })
})
