import { describe, expect, it } from 'vitest'
import { extractSIWCCallback } from '../siwcCallback'

const want = 'http://127.0.0.1:1455/auth/callback?code=code&state=state&client_id=issued'

describe('extractSIWCCallback', () => {
  it('accepts a full paste, the URL, or only the query', () => {
    expect(extractSIWCCallback(`无法访问此网站\nhttp://localhost:1455/auth/callback?code=code&state=state&client_id=issued\nERR_CONNECTION_REFUSED`)).toBe(want)
    expect(extractSIWCCallback(want)).toBe(want)
    expect(extractSIWCCallback('?code=code&state=state&client_id=issued')).toBe(want)
    expect(extractSIWCCallback('https://evil.example/auth/callback?code=code&state=state&client_id=issued')).toBeNull()
  })
})
