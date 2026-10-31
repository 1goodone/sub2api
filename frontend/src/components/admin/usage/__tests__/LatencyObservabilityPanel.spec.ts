import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import LatencyObservabilityPanel from '../LatencyObservabilityPanel.vue'
import en from '@/i18n/locales/en/latencyObservability'
import zh from '@/i18n/locales/zh/latencyObservability'
const mocks = vi.hoisted(() => ({ read: vi.fn() }))
vi.mock('@/api/admin/latencyObservability', () => ({ getLatencyReport: mocks.read }))
const group = { body_bucket: '1-4MiB', cache_bucket: '95%+', reasoning_effort: 'xhigh', requests: 2, traces: 2, attempts: 3, reasoning_samples: 1, reasoning_first_avg_ms: 120, text_samples: 1, visible_text_first_avg_ms: 450, byte_samples: 1, response_byte_first_avg_ms: 30, upload_samples: 1, wait_samples: 0, visible_samples: 0, body_upload_avg_ms: 0, post_upload_first_content_wait_avg_ms: null, post_upload_first_visible_wait_avg_ms: null, large_body_switches: 1, large_body_same_account_retries: 1, failover_samples: 1, cost_samples: 0, extra_upload_component_ms: null, extra_wait_component_ms: null }
const result = { groups: [group], end_to_end_ms: null, end_to_end_status: 'not_collected', end_to_end_source: 'none', client_upload_ms: null, client_upload_status: 'not_collected' }
describe('Issue603 latency observations', () => {
  for (const [locale, messages] of Object.entries({ en, zh })) {
    it(`omits client timing while retaining server observations in ${locale}`, async () => {
      mocks.read.mockResolvedValue(result)
      const w = mount(LatencyObservabilityPanel, { props: { model: 'fixture', accountId: 4 }, global: { plugins: [createI18n({ legacy: false, locale, messages: { [locale]: { latencyObservability: messages } } })] } })
      await flushPromises()
      expect(w.text()).toContain('0.0 ms (n=1)')
      expect(w.text()).toContain(`${messages.notCollected} (n=0)`)
      expect(w.text()).not.toMatch(/Client end-to-end time|Client upload before application entry|客户端端到端耗时|进入应用前的客户端上传/)
      expect(w.find('[data-testid="reasoning-capability"]').exists()).toBe(false)
      expect(w.text()).toContain('1 / 1 (n=1)')
      expect(w.text()).toContain(messages.reasoning_first_ms)
      expect(w.text()).toContain('120.0 ms (n=1)')
      expect(w.text()).toContain(messages.visible_text_first_ms)
      expect(w.text()).toContain('450.0 ms (n=1)')
      expect(w.text()).toContain(messages.response_byte_first_ms)
      expect(w.text()).toContain('30.0 ms (n=1)')
      expect(mocks.read).toHaveBeenLastCalledWith(false, expect.objectContaining({ account_id: 4, model: 'fixture' }), expect.any(AbortSignal))
      w.unmount()
    })
  }
  it('rejects stale report responses when filters change', async () => {
    let resolve!: (v: unknown) => void
    mocks.read.mockImplementationOnce(() => new Promise(r => { resolve = r })).mockResolvedValueOnce({ ...result, groups: [] })
    const w = mount(LatencyObservabilityPanel, { props: { model: 'old' }, global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: { latencyObservability: en } } })] } })
    await w.setProps({ model: 'new', ops: true }); await flushPromises()
    resolve(result); await flushPromises()
    expect(w.find('[data-testid="latency-group"]').exists()).toBe(false)
    expect(mocks.read).toHaveBeenLastCalledWith(true, expect.objectContaining({ model: 'new' }), expect.any(AbortSignal))
    w.unmount()
  })
})

it('omits unavailable reasoning capability and retains per-hop socket evidence scope', async () => {
  mocks.read.mockResolvedValue(result)
  const w = mount(LatencyObservabilityPanel, { props: { ops: true }, global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: { latencyObservability: en } } })] } })
  await flushPromises()
  expect(w.find('[data-testid="reasoning-capability"]').exists()).toBe(false)
  expect(w.get('[data-testid="socket-evidence"]').text()).toContain('source none')
  expect(w.get('[data-testid="socket-evidence"]').text()).toContain('reverse proxy')
  expect(w.text()).toContain(en.typedScope)
  w.unmount()
})
