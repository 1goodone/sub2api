import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import AccountLoginStateCell from '../AccountLoginStateCell.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/accountTokenGuardV2', () => ({ reloginTokenGuardV2Account: vi.fn() }))
vi.mock('@/api/admin/siwc', () => ({ loginSIWCWithSavedCredentials: vi.fn() }))

const account = { id: 7, platform: 'openai', type: 'oauth', name: 'paid' }
const guard = {
  login_config: { configured: true },
  probe_state: 'ok',
  latest_task: { id: 9, account_id: 7, status: 'failed', oauth_profile: 'excel', stage: 'done', attempt: 1, created_at: '', updated_at: '' }
}

describe('AccountLoginStateCell', () => {
  it('keeps a healthy credential normal when the latest BPS login failed', () => {
    const wrapper = mount(AccountLoginStateCell, { props: { kind: 'credential', account: account as never, guard: guard as never } })
    expect(wrapper.get('[data-testid="login-column-credential"]').text()).toContain('admin.accounts.loginColumns.ok')
    expect(wrapper.text()).not.toContain('admin.accounts.loginColumns.relogin')
    wrapper.unmount()
  })

  it('opens the BPS proxy dialog signal from the relogin button', async () => {
    const wrapper = mount(AccountLoginStateCell, {
      props: {
        kind: 'bps',
        account: account as never,
        bps: { task: { status: 'failed', error_code: 'security_check' } } as never
      }
    })
    const button = wrapper.get('button')
    expect(button.text()).toContain('admin.accounts.loginColumns.relogin')
    await button.trigger('pointerdown')
    await button.trigger('click')
    expect(wrapper.emitted('add')?.length).toBeGreaterThan(0)
    wrapper.unmount()
  })

  it('still shows credential relogin when the Codex login itself failed', () => {
    const failed = { ...guard, latest_task: { ...guard.latest_task, oauth_profile: 'codex' } }
    const wrapper = mount(AccountLoginStateCell, { props: { kind: 'credential', account: account as never, guard: failed as never } })
    expect(wrapper.get('[data-testid="login-column-credential"]').text()).toContain('admin.accounts.loginColumns.relogin')
    wrapper.unmount()
  })
})
