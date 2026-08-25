import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { testAccount } = vi.hoisted(() => ({ testAccount: vi.fn() }))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { testAccount } }
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key
  })
}))

import AccountBatchTestModal from '../AccountBatchTestModal.vue'

const accounts = [
  { id: 1, name: 'First', platform: 'openai' },
  { id: 2, name: 'Second', platform: 'anthropic' }
] as any[]

const deferred = <T,>() => {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const mountModal = () => mount(AccountBatchTestModal, {
  props: { show: true, accountIds: [1, 2], accounts },
  global: {
    stubs: {
      BaseDialog: {
        props: ['show', 'title'],
        emits: ['close'],
        template: '<div v-if="show"><slot /><slot name="footer" /></div>'
      }
    }
  }
})

describe('AccountBatchTestModal', () => {
  beforeEach(() => testAccount.mockReset())

  it('tests selected accounts strictly one at a time', async () => {
    const first = deferred<{ success: boolean; model?: string }>()
    const second = deferred<{ success: boolean; model?: string }>()
    testAccount.mockImplementationOnce(() => first.promise).mockImplementationOnce(() => second.promise)
    const wrapper = mountModal()

    await wrapper.get('[data-testid="start-batch-test"]').trigger('click')
    expect(testAccount).toHaveBeenCalledTimes(1)
    expect(testAccount.mock.calls[0][0]).toBe(1)
    expect(testAccount.mock.calls[0][1]).toEqual({ prompt: 'hi' })

    first.resolve({ success: false, model: 'first-model' })
    await flushPromises()
    expect(testAccount).toHaveBeenCalledTimes(2)
    expect(testAccount.mock.calls[1][0]).toBe(2)

    second.resolve({ success: true })
    await flushPromises()
    const items = (wrapper.vm as any).items
    expect(items.map((item: any) => item.status)).toEqual(['failed', 'success'])
    wrapper.unmount()
  })

  it('aborts the active request and marks remaining accounts stopped', async () => {
    const active = deferred<{ success: boolean }>()
    testAccount.mockReturnValue(active.promise)
    const wrapper = mountModal()

    await wrapper.get('[data-testid="start-batch-test"]').trigger('click')
    const signal = testAccount.mock.calls[0][2].signal as AbortSignal
    await wrapper.get('[data-testid="stop-batch-test"]').trigger('click')

    expect(signal?.aborted).toBe(true)
    expect((wrapper.vm as any).items.map((item: any) => item.status)).toEqual(['cancelled', 'cancelled'])

    active.resolve({ success: false })
    await flushPromises()
    expect(testAccount).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('aborts the active request when the dialog is unmounted', async () => {
    const active = deferred<{ success: boolean }>()
    testAccount.mockReturnValue(active.promise)
    const wrapper = mountModal()

    await wrapper.get('[data-testid="start-batch-test"]').trigger('click')
    const signal = testAccount.mock.calls[0][2].signal as AbortSignal
    wrapper.unmount()

    expect(signal.aborted).toBe(true)
    active.resolve({ success: false })
    await flushPromises()
    expect(testAccount).toHaveBeenCalledTimes(1)
  })
})
