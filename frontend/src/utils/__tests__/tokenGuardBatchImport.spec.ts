import { describe, expect, it } from 'vitest'
import { parseTokenGuardBatchImport } from '../tokenGuardBatchImport'

describe('parseTokenGuardBatchImport', () => {
  it('keeps the last row for a repeated email and accepts both formats', () => {
    const parsed = parseTokenGuardBatchImport([
      'User@Example.com----old----OLD',
      'user@example.com----new----NEW',
      'backup@example.com----https://otp.example.com/latest',
    ].join('\n'))

    expect(parsed.issues).toEqual([])
    expect(parsed.rows).toEqual([
      { email: 'user@example.com', mode: 'password_totp', password: 'new', totp: 'NEW' },
      { email: 'backup@example.com', mode: 'email_otp_url', otpUrl: 'https://otp.example.com/latest' },
    ])
  })

  it('rejects a bad line without returning rows', () => {
    const parsed = parseTokenGuardBatchImport('ok@example.com----secret----2fa\nnot-an-email----https://otp.example.com')
    expect(parsed.rows).toEqual([])
    expect(parsed.issues).toEqual([{ line: 2, kind: 'email' }])
  })
})
