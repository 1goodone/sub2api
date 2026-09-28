import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'

import TokenGuardV2View from '../TokenGuardV2View.vue'

const api = vi.hoisted(() => ({
  listGuard: vi.fn(),
  createGuard: vi.fn(),
  updateGuard: vi.fn(),
  deleteGuard: vi.fn(),
  probeGuard: vi.fn(),
  reloginGuard: vi.fn(),
  listAccounts: vi.fn(),
  listProxies: vi.fn(),
}))

vi.mock('@/api/admin/accountTokenGuardV2', () => ({
  listTokenGuardV2Accounts: api.listGuard,
  createTokenGuardV2Account: api.createGuard,
  updateTokenGuardV2Account: api.updateGuard,
  deleteTokenGuardV2Account: api.deleteGuard,
  probeTokenGuardV2Account: api.probeGuard,
  reloginTokenGuardV2Account: api.reloginGuard,
}))
vi.mock('@/api/admin/accounts', () => ({ list: api.listAccounts, default: { list: api.listAccounts } }))
vi.mock('@/api/admin/proxies', () => ({
  getAllWithCount: api.listProxies,
  default: { getAllWithCount: api.listProxies },
}))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => params ? `${key} ${Object.values(params).join(' ')}` : key,
    }),
  }
})

let wrapper: VueWrapper | undefined

beforeEach(() => {
  vi.clearAllMocks()
  api.listGuard.mockResolvedValue({
    probe_interval_seconds: 1800,
    fail_streak_threshold: 2,
    accounts: [{
      account_id: 42,
      account_name: 'Primary',
      account_status: 'active',
      schedulable: true,
      enabled: true,
      auto_relogin_enabled: true,
      probe_state: 'ok',
      probe_detail: 'credential accepted',
      fail_streak: 0,
      next_probe_at: '2026-09-28T10:00:00Z',
      login_config: {
        account_id: 42,
        login_email: 'user@example.com',
        credential_mode: 'password_totp',
        proxy_source: 'managed_proxy',
        proxy_id: 7,
        password_configured: true,
        totp_configured: true,
        configured: true,
      },
    }],
  })
  api.listAccounts.mockResolvedValue({ items: [
    { id: 42, name: 'Primary' },
    { id: 43, name: 'Backup', extra: { email_address: 'backup@example.com' } },
  ] })
  api.listProxies.mockResolvedValue([
    { id: 7, name: 'Tokyo', protocol: 'socks5h', host: '203.0.113.7', port: 1080 },
    { id: 8, name: 'Singapore', protocol: 'http', host: '198.51.100.8', port: 8080 },
  ])
  api.updateGuard.mockResolvedValue({})
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
})

describe('TokenGuardV2View', () => {
  it('supports both per-account login modes without echoing saved secrets', async () => {
    wrapper = mount(TokenGuardV2View, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          SmartOpsNav: true,
          Icon: true,
          BaseDialog: {
            props: ['show'],
            template: '<section v-if="show"><slot /><footer><slot name="footer" /></footer></section>',
          },
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).not.toContain('password-secret')
    expect(wrapper.text()).not.toContain('totp-secret')
    await wrapper.findAll('button').find(button => button.text() === 'tokenGuardV2.edit')!.trigger('click')

    const modes = wrapper.findAll('input[type="radio"]').map(input => input.attributes('value'))
    expect(modes).toEqual(['password_totp', 'email_otp_url'])
    expect(wrapper.findAll('input[type="password"]').map(input => input.element.value)).toEqual(['', ''])

    expect(wrapper.text()).toContain('SOCKS5H')
    await wrapper.findAll('select')[1].setValue('proxy:8')

    const saveButton = wrapper.findAll('button').find(button => button.text() === 'tokenGuardV2.save')!
    expect(saveButton.attributes()).toMatchObject({ type: 'submit', form: 'token-guard-v2-editor' })
    await wrapper.get('#token-guard-v2-editor').trigger('submit')
    await flushPromises()
    expect(api.updateGuard).toHaveBeenCalledWith(42, expect.objectContaining({
      credential_mode: 'password_totp',
      proxy_source: 'managed_proxy',
      proxy_id: 8,
      password: '',
      totp_secret: '',
    }))
  })

  it('saves the managed Mihomo pool without a proxy id', async () => {
    wrapper = mount(TokenGuardV2View, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          SmartOpsNav: true,
          Icon: true,
          BaseDialog: {
            props: ['show'],
            template: '<section v-if="show"><slot /><footer><slot name="footer" /></footer></section>',
          },
        },
      },
    })
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'tokenGuardV2.edit')!.trigger('click')
    await wrapper.findAll('select')[1].setValue('mihomo')
    await wrapper.get('#token-guard-v2-editor').trigger('submit')
    await flushPromises()

    expect(api.updateGuard).toHaveBeenCalledWith(42, expect.objectContaining({
      proxy_source: 'mihomo',
      proxy_id: null,
    }))
  })

  it('fills the login email from the selected managed account', async () => {
    wrapper = mount(TokenGuardV2View, {
      global: {
        stubs: {
          AppLayout: { template: '<main><slot /></main>' },
          SmartOpsNav: true,
          Icon: true,
          BaseDialog: {
            props: ['show'],
            template: '<section v-if="show"><slot /><footer><slot name="footer" /></footer></section>',
          },
        },
      },
    })
    await flushPromises()

    await wrapper.findAll('button').find(button => button.text() === 'tokenGuardV2.addAccount')!.trigger('click')
    await wrapper.get('select').setValue('43')

    expect((wrapper.get('input[type="email"]').element as HTMLInputElement).value).toBe('backup@example.com')
  })
})
