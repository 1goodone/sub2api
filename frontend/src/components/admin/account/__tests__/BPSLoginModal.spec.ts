import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'

import BPSLoginModal from '../BPSLoginModal.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { startBPSLogin: vi.fn() } }
}))

const account = {
  id: 7,
  name: 'paid',
  platform: 'openai',
  type: 'oauth',
  credentials: { plan_type: 'plus', auth_mode: 'oauth' }
}

describe('BPSLoginModal', () => {
  it('renders the proxy field on document.body when opened', async () => {
    const wrapper = mount(BPSLoginModal, {
      props: { show: true, accounts: [account as never], selectedIds: [7] },
      attachTo: document.body,
      global: { stubs: { Icon: true } }
    })

    await wrapper.vm.$nextTick()
    const shell = document.body.querySelector('[data-testid="bps-login-dialog"]') as HTMLElement | null
    const dialog = document.body.querySelector('[data-testid="bps-login-modal"]')
    const proxy = document.body.querySelector('[data-testid="bps-login-proxy"]')
    expect(shell).not.toBeNull()
    expect(shell?.tagName).toBe('DIV')
    expect(shell?.style.display).toBe('flex')
    expect(shell?.style.position).toBe('fixed')
    expect('showModal' in (shell as HTMLElement)).toBe(false)
    expect(dialog).not.toBeNull()
    expect(proxy).not.toBeNull()
    expect(document.body.contains(dialog)).toBe(true)

    shell?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('close')).toBeUndefined()
    wrapper.unmount()
  })
})
