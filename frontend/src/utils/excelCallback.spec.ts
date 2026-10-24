import { describe, expect, it } from 'vitest'
import { extractExcelCallback } from './excelCallback'

const callback = 'https://bps.openai.com/basispoints/extension/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3/auth/callback?code=abc&state=bps.test.PC'

describe('extractExcelCallback', () => {
  it('accepts the official callback and a browser error page that contains it', () => {
    expect(extractExcelCallback(callback)).toBe(callback)
    expect(extractExcelCallback(`无法完成登录\n${callback}`)).toBe(callback)
  })

  it('rejects localhost and other callback hosts', () => {
    expect(extractExcelCallback('http://localhost:1455/auth/callback?code=abc&state=bps.test.PC')).toBeNull()
    expect(extractExcelCallback('https://example.com/auth/callback?code=abc&state=bps.test.PC')).toBeNull()
  })
})
