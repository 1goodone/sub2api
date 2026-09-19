import { mount, flushPromises, type VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import en from '@/i18n/locales/en'
import CodexStatePoolView from '../CodexStatePoolView.vue'

const api = vi.hoisted(() => ({ getStatePool: vi.fn(), saveStatePoolProxy: vi.fn(), refreshState: vi.fn(), getStateValue: vi.fn(), getStateEvents: vi.fn(), importState: vi.fn(), getImportJob: vi.fn(), copy: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string, values: Record<string, unknown> = {}) => {
  const message = key.split('.').reduce<unknown>((value, part) => (value as Record<string, unknown>)?.[part], en)
  return String(message ?? key).replace(/\{(\w+)\}/g, (_, name: string) => String(values[name] ?? ''))
} }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
vi.mock('@/components/common/ProxySelector.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/api/admin/codexStatePool', () => api)
vi.mock('@/api/admin/proxies', () => ({ getAll: vi.fn().mockResolvedValue([]), default: {} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: api.success, showError: api.error }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: api.copy }) }))
let wrapper: VueWrapper | undefined
function data() {
  const now = Date.now()
  const row = { account_id: 1, account_name: 'Team account', account_status: 'active', plan: 'team', model: 'gpt-6-astra', enabled: true, expected_length: 332, state_length: 332, state_status: 'valid', obtained_at: new Date(now).toISOString(), expires_at: new Date(now + 3600000).toISOString(), probe_status: 'stopped', attempts: 25, can_refresh: true, reason: 'probe_failed' }
  return { server_time: new Date(now).toISOString(), config: { proxy_id: 7, proxy_name: 'Shared proxy', available: true, issue: '' }, models: ['gpt-6-astra', 'gpt-5.5'], items: [row, { ...row, model: 'gpt-5.5' }, { ...row, account_id: 2, account_name: 'Other account', model: 'gpt-5.5' }] }
}
async function render() {
  wrapper = mount(CodexStatePoolView, { global: { stubs: { AppLayout: { template: '<main><slot /></main>' }, RouterLink: { template: '<a><slot /></a>' }, ProxySelector: { template: '<div />' } } } })
  await flushPromises()
  return wrapper
}
beforeEach(() => { vi.clearAllMocks(); api.getStatePool.mockResolvedValue(data()); api.refreshState.mockResolvedValue(undefined); api.getStateValue.mockResolvedValue('t'.repeat(332)); api.copy.mockResolvedValue(true); api.getStateEvents.mockResolvedValue([]); api.importState.mockResolvedValue({ id: 'import-1', account_id: 1, model: 'gpt-6-astra', status: 'queued', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }); api.getImportJob.mockResolvedValue({ id: 'import-1', account_id: 1, model: 'gpt-6-astra', status: 'succeeded', created_at: new Date().toISOString(), updated_at: new Date().toISOString() }) })
afterEach(() => { wrapper?.unmount() })
describe('Codex state pool', () => {
  it('defaults to astra and keeps a valid candidate healthy after failed renewal', async () => {
    const view = await render()
    expect(view.find('select').element.value).toBe('gpt-6-astra')
    expect(view.findAll('article')).toHaveLength(2)
    expect(view.text()).toContain('332 healthy candidate')
    expect(view.text()).toContain('Attempt limit reached')
    const second = view.findAll('select')[1]!
    expect(second.element.value).toBe('gpt-5.5')
    await view.find('select').setValue('gpt-5.5')
    expect(second.element.value).toBe('gpt-5.5')
    expect(view.findAll('article')).toHaveLength(2)
  })
  it('fetches raw state only for an explicit copy action', async () => {
    const view = await render()
    expect(api.getStateValue).not.toHaveBeenCalled()
    await view.get('button[aria-label="Copy State"]').trigger('click')
    await flushPromises()
    expect(api.copy).toHaveBeenCalledWith('t'.repeat(332), 'State copied')
    expect(view.text()).not.toContain('t'.repeat(332))
  })
  it('queues manual refresh without removing the current valid card', async () => {
    const view = await render()
    await view.findAll('button').find(button => button.text() === 'Probe and refresh')!.trigger('click')
    await flushPromises()
    expect(api.refreshState).toHaveBeenCalledWith(expect.objectContaining({ account_id: 1, model: 'gpt-6-astra' }))
    expect(view.text()).toContain('332 healthy candidate')
  })
  it('disables manual refresh when the backend marks the row unavailable', async () => {
    const payload = data()
    payload.items[0]!.can_refresh = false
    api.getStatePool.mockResolvedValueOnce(payload)
    const view = await render()
    const refresh = view.findAll('button').find(button => button.text() === 'Probe and refresh')!

    expect(refresh.attributes('disabled')).toBeDefined()
    await refresh.trigger('click')
    expect(api.refreshState).not.toHaveBeenCalled()
  })
  it('validates pasted state for the selected account/model and clears it on success', async () => {
    const view = await render()
    await view.findAll('button').find(button => button.text() === 'Paste State')!.trigger('click')
    await view.find('textarea').setValue('  pasted-state  ')
    await view.findAll('button').find(button => button.text() === 'Validate and replace')!.trigger('click')
    await flushPromises()
    expect(api.importState).toHaveBeenCalledWith(expect.objectContaining({ account_id: 1, model: 'gpt-6-astra' }), 'pasted-state')
    expect(view.find('textarea').exists()).toBe(false)
  })

  it('shows disabled and expired candidates without healthy styling or copy access', async () => {
    const payload = data()
    payload.items[0]!.state_status = 'disabled'
    payload.items[0]!.can_refresh = false
    payload.items[2]!.expires_at = new Date(Date.now() - 1000).toISOString()
    api.getStatePool.mockResolvedValueOnce(payload)
    const view = await render()
    const cards = view.findAll('article')
    expect(cards[0]!.text()).toContain('State Pool mode not selected')
    expect(cards[0]!.classes()).toContain('border-l-gray-400')
    expect(cards[1]!.text()).toContain('Expired')
    expect(cards[1]!.classes()).toContain('border-l-red-500')
    for (const button of view.findAll('button[aria-label="Copy State"]')) expect(button.attributes('disabled')).toBeDefined()
  })
  it('loads logs separately for each selected model', async () => {
    const view = await render()
    await view.findAll('button').find(button => button.text().includes('Probe and invalidation logs'))!.trigger('click')
    await flushPromises()
    expect(api.getStateEvents).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 1, model: 'gpt-6-astra' }), 0)
    await view.find('select').setValue('gpt-5.5')
    await flushPromises()
    expect(api.getStateEvents).toHaveBeenLastCalledWith(expect.objectContaining({ account_id: 1, model: 'gpt-5.5' }), 0)
  })

  it('shows normalized API rejection reasons and preserves the valid card', async () => {
    api.importState.mockRejectedValueOnce({ message: 'not_newer' })
    const view = await render()
    await view.findAll('button').find(button => button.text() === 'Paste State')!.trigger('click')
    await view.find('textarea').setValue('old-state')
    await view.findAll('button').find(button => button.text() === 'Validate and replace')!.trigger('click')
    await flushPromises()
    expect(view.get('[role="alert"]').text()).toContain('Expiry not improved')
    expect(view.text()).toContain('332 healthy candidate')
    expect(view.find('textarea').exists()).toBe(true)
  })

})
