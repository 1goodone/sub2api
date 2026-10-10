export type TokenGuardBatchRow =
  | { email: string; mode: 'password_totp'; password: string; totp: string }
  | { email: string; mode: 'email_otp_url'; otpUrl: string }

export type TokenGuardBatchIssue = { line: number; kind: 'format' | 'email' | 'url' }

const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

export function parseTokenGuardBatchImport(text: string): { rows: TokenGuardBatchRow[]; issues: TokenGuardBatchIssue[] } {
  const issues: TokenGuardBatchIssue[] = []
  const order: string[] = []
  const byEmail = new Map<string, TokenGuardBatchRow>()
  text.split(/\r?\n/).forEach((raw, index) => {
    const line = raw.trim()
    if (!line) return
    const lineNumber = index + 1
    const parts = line.split('----').map(part => part.trim())
    if (parts.length < 2 || parts.some(part => part === '' && parts.length === 2)) {
      issues.push({ line: lineNumber, kind: 'format' })
      return
    }
    const email = parts[0]
    if (!emailPattern.test(email)) {
      issues.push({ line: lineNumber, kind: 'email' })
      return
    }
    let row: TokenGuardBatchRow
    if (parts.length === 2) {
      if (!/^https?:\/\//i.test(parts[1])) {
        issues.push({ line: lineNumber, kind: 'url' })
        return
      }
      row = { email, mode: 'email_otp_url', otpUrl: parts[1] }
    } else {
      const totp = parts[parts.length - 1]
      const password = parts.slice(1, -1).join('----')
      if (!password) {
        issues.push({ line: lineNumber, kind: 'format' })
        return
      }
      row = { email, mode: 'password_totp', password, totp }
    }
    const key = email.toLowerCase()
    if (!byEmail.has(key)) order.push(key)
    byEmail.set(key, row)
  })
  if (issues.length) return { rows: [], issues }
  return { rows: order.map(key => byEmail.get(key)!), issues }
}
