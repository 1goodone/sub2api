const EXCEL_CALLBACK_ORIGIN = 'https://bps.openai.com'
const EXCEL_CALLBACK_PATH = '/basispoints/extension/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3/auth/callback'

export function extractExcelCallback(raw: string): string | null {
  const match = raw.match(/https:\/\/bps\.openai\.com\/basispoints\/extension\/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3\/auth\/callback\?[^\s"'<>]+/i)
  if (!match) return null
  try {
    const url = new URL(match[0])
    if (url.origin !== EXCEL_CALLBACK_ORIGIN || url.pathname !== EXCEL_CALLBACK_PATH || url.username || url.password || url.hash) {
      return null
    }
    if (!url.searchParams.get('state') || (!url.searchParams.get('code') && !url.searchParams.get('error'))) {
      return null
    }
    url.hash = ''
    return url.toString()
  } catch {
    return null
  }
}
