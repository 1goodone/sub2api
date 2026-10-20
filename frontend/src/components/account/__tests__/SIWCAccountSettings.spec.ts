import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import SIWCAccountSettings from '../SIWCAccountSettings.vue'
import { authorizeSIWC, completeSIWC, getSIWCStatus, loginSIWCWithSavedCredentials } from '@/api/admin/siwc'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/siwc', () => ({
  authorizeSIWC: vi.fn(), completeSIWC: vi.fn(), getSIWCStatus: vi.fn(), loginSIWCWithSavedCredentials: vi.fn()
}))
const props = { accountId: 42, enabled: false, active: true }
beforeEach(() => { vi.resetAllMocks(); vi.mocked(getSIWCStatus).mockResolvedValue({ bound: false, enabled: false }) })
describe('SIWC account protocol and authorization', () => {
  it('requires a grant before switching and keeps selection explicit', async () => {
    const wrapper = mount(SIWCAccountSettings, { props })
    await flushPromises()
    expect(wrapper.get('option[value="siwc"]').attributes('disabled')).toBeDefined()
    vi.mocked(getSIWCStatus).mockResolvedValue({ bound: true, enabled: false })
    await wrapper.setProps({ active: false }); await wrapper.setProps({ active: true }); await flushPromises()
    expect(wrapper.get('option[value="siwc"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.emitted('update:enabled')).toBeUndefined()
    await wrapper.get('[data-testid="siwc-protocol"]').setValue('siwc')
    expect(wrapper.emitted('update:enabled')).toEqual([[true]])
    wrapper.unmount()
  })
  it('queues a SIWC login using saved credentials without requesting passwords', async () => {
    vi.mocked(loginSIWCWithSavedCredentials).mockResolvedValue()
    const wrapper = mount(SIWCAccountSettings, { props }); await flushPromises()
    await wrapper.get('[data-testid="siwc-auto"]').trigger('click'); await flushPromises()
    expect(loginSIWCWithSavedCredentials).toHaveBeenCalledWith(42)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('binds the browser callback to its account and clears secrets when closed', async () => {
    vi.mocked(authorizeSIWC).mockResolvedValue({ session_id: 'session-42', auth_url: 'https://auth.openai.com/api/accounts/authorize' })
    vi.mocked(completeSIWC).mockResolvedValue()
    const wrapper = mount(SIWCAccountSettings, { props }); await flushPromises()
    await wrapper.get('[data-testid="siwc-browser"]').trigger('click'); await flushPromises()
    await wrapper.get('input').setValue('http://127.0.0.1:1455/auth/callback?code=test&state=test')
    await wrapper.setProps({ active: false }); await flushPromises()
    expect(wrapper.find('[data-testid="siwc-browser-flow"]').exists()).toBe(false)
    expect(completeSIWC).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
