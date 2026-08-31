import { defineComponent } from 'vue'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import DesktopConnectView from '../DesktopConnectView.vue'

const mocks = vi.hoisted(() => ({
  post: vi.fn(),
  query: { request: 'a'.repeat(43) }
}))

vi.mock('@/api/client', () => ({ apiClient: { post: mocks.post } }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: mocks.query }) }))
vi.mock('@/components/layout', () => ({
  AuthLayout: defineComponent({ template: '<main><slot /></main>' })
}))

describe('DesktopConnectView', () => {
  beforeEach(() => mocks.post.mockReset())

  it('approves the matching desktop request without asking for credentials', async () => {
    mocks.post.mockResolvedValue({ data: { status: 'approved' } })
    const wrapper = mount(DesktopConnectView, {
      global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } }
    })

    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('AAAA-AAAA')
    await wrapper.get('button').trigger('click')
    await vi.waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/auth/desktop/approve', { request_id: 'a'.repeat(43) }))
    expect(wrapper.text()).toContain('桌面端已授权')
  })
})
