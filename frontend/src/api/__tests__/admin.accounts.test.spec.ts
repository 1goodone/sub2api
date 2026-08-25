import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/api/client', () => ({
  apiClient: {},
  buildApiUrl: (path: string) => path
}))

import { testAccount } from '@/api/admin/accounts'

describe('admin account connectivity test API', () => {
  beforeEach(() => {
    vi.restoreAllMocks()
    localStorage.clear()
  })

  it('parses chunked SSE and returns the terminal result', async () => {
    localStorage.setItem('auth_token', 'test-token')
    const encoder = new TextEncoder()
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode('data: {"type":"test_start","model":"gpt-test"}\n'))
        controller.enqueue(encoder.encode('\ndata:{"type":"test_complete","success":true}\n\n'))
        controller.close()
      }
    })
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(stream, { status: 200 }))
    const onEvent = vi.fn()

    const result = await testAccount(42, { prompt: 'hi' }, { onEvent })

    expect(result).toEqual({ success: true, model: 'gpt-test', error: undefined })
    expect(onEvent).toHaveBeenCalledTimes(2)
    expect(fetchMock).toHaveBeenCalledWith('/admin/accounts/42/test', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ prompt: 'hi' }),
      headers: expect.objectContaining({ Authorization: 'Bearer test-token' })
    }))
  })

  it('returns an SSE error as a failed result', async () => {
    const encoder = new TextEncoder()
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(encoder.encode('data: not-json\n\ndata: {"type":"error","error":"quota reached"}\n\n'))
        controller.close()
      }
    })
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(stream, { status: 200 }))

    await expect(testAccount(7)).resolves.toEqual({
      success: false,
      model: undefined,
      error: 'quota reached'
    })
  })
})
