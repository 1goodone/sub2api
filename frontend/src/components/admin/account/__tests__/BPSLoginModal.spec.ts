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
      attachTo: document.body
    })

    await wrapper.vm.$nextTick()
    const dialog = document.body.querySelector('[data-testid="bps-login-modal"]')
    const proxy = document.body.querySelector('[data-testid="bps-login-proxy"]')
    expect(dialog).not.toBeNull()
    expect(proxy).not.toBeNull()
    expect(document.body.contains(dialog)).toBe(true)
    wrapper.unmount()
  })
})
