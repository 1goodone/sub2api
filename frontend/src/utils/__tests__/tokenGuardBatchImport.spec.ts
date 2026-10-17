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

  it('accepts three-dash rows and still prefers four dashes when a password contains three', () => {
    const parsed = parseTokenGuardBatchImport([
      'user@example.com---secret---NEW',
      'backup@example.com---https://otp.example.com/latest',
      'dashed@example.com----pa---ss----TOKEN',
    ].join('\n'))

    expect(parsed.issues).toEqual([])
    expect(parsed.rows).toEqual([
      { email: 'user@example.com', mode: 'password_totp', password: 'secret', totp: 'NEW' },
      { email: 'backup@example.com', mode: 'email_otp_url', otpUrl: 'https://otp.example.com/latest' },
      { email: 'dashed@example.com', mode: 'password_totp', password: 'pa---ss', totp: 'TOKEN' },
    ])
  })

  it('rejects a bad line without returning rows', () => {
    const parsed = parseTokenGuardBatchImport('ok@example.com----secret----2fa\nnot-an-email----https://otp.example.com')
    expect(parsed.rows).toEqual([])
    expect(parsed.issues).toEqual([{ line: 2, kind: 'email' }])
  })
})
