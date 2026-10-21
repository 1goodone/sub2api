const callbackPattern = /https?:\/\/(?:127\.0\.0\.1|localhost):1455\/auth\/callback(?:\?[^\s"'<>`]*)?/i

// Pulls the loopback callback out of a full browser paste, or accepts that URL
// or its query string alone. localhost is rewritten to 127.0.0.1.
export function extractSIWCCallback(raw: string): string | null {
  const text = raw.trim().replace(/^["'`]+|["'`]+$/g, '')
  const found = text.match(callbackPattern)
  if (found && found[0].toLowerCase().startsWith('http://')) {
    const cleaned = found[0].replace(/[.,;，。；)）\]]+$/g, '')
    try {
      const url = new URL(cleaned)
      if (url.pathname === '/auth/callback' && (url.hostname === '127.0.0.1' || url.hostname === 'localhost') && url.port === '1455' && url.username === '') {
        return `http://127.0.0.1:1455/auth/callback${url.search}`
      }
    } catch {
      return null
    }
  }
  const query = text.replace(/^\?/, '')
  if (query.includes('code=') && query.includes('state=') && !/[\s/\\]/.test(query) && !/http/i.test(text)) {
    return `http://127.0.0.1:1455/auth/callback?${query}`
  }
  return null
}
