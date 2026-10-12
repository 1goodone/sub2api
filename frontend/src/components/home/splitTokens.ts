export interface SplitToken { text: string; index: number; space: boolean; highlight: boolean }

const CJK = /[⺀-鿿＀-￯]/

/**
 * Splits copy into animatable tokens: characters for CJK text, words otherwise.
 * `[[...]]` marks highlighted runs. Whitespace tokens get index -1.
 */
export function splitTokens(text: string): SplitToken[] {
  const cjk = CJK.test(text)
  const tokens: SplitToken[] = []
  let index = 0
  for (const part of text.split(/(\[\[.*?\]\])/)) {
    if (!part) continue
    const highlight = part.startsWith('[[') && part.endsWith(']]')
    const body = highlight ? part.slice(2, -2) : part
    const pieces = cjk ? Array.from(body) : body.split(/(\s+)/).filter(Boolean)
    for (const piece of pieces) {
      const space = /^\s+$/.test(piece)
      tokens.push({ text: piece, index: space ? -1 : index++, space, highlight })
    }
  }
  return tokens
}
