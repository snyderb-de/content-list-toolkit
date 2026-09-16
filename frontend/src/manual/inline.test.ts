import { describe, expect, it } from 'vitest'
import { parseInline } from './inline'

describe('parseInline', () => {
  it('returns plain text as one segment', () => {
    expect(parseInline('just words')).toEqual([{ kind: 'text', value: 'just words' }])
  })

  it('reads **bold**', () => {
    expect(parseInline('a **b** c')).toEqual([
      { kind: 'text', value: 'a ' },
      { kind: 'bold', value: 'b' },
      { kind: 'text', value: ' c' },
    ])
  })

  it('reads `code`', () => {
    expect(parseInline('run `npm test` now')).toEqual([
      { kind: 'text', value: 'run ' },
      { kind: 'code', value: 'npm test' },
      { kind: 'text', value: ' now' },
    ])
  })

  it('reads several markers in one string', () => {
    expect(parseInline('**Fixed** means `done`')).toEqual([
      { kind: 'bold', value: 'Fixed' },
      { kind: 'text', value: ' means ' },
      { kind: 'code', value: 'done' },
    ])
  })

  // Markup is never handed to a renderer as HTML, so content containing angle
  // brackets stays content. This is the property both renderers rely on.
  it('treats markup characters in the content as text', () => {
    expect(parseInline('use <strong> carefully')).toEqual([
      { kind: 'text', value: 'use <strong> carefully' },
    ])
  })

  it('leaves an unclosed marker as text rather than swallowing the rest', () => {
    expect(parseInline('a **b')).toEqual([{ kind: 'text', value: 'a **b' }])
    expect(parseInline('a `b')).toEqual([{ kind: 'text', value: 'a `b' }])
  })

  it('leaves empty markers as text', () => {
    // "****" has nothing to emphasise, so treating it as bold would render
    // nothing at all and silently lose the characters.
    expect(parseInline('****')).toEqual([{ kind: 'text', value: '****' }])
  })

  it('handles an empty string', () => {
    expect(parseInline('')).toEqual([])
  })

  // Every segment's value concatenated must equal the input minus its markers,
  // or the renderers would drop characters from the manual.
  it('never loses content', () => {
    for (const text of [
      'plain',
      'a **b** c `d` e',
      '**start** and end `x`',
      'Windows path `X:\\Apps` in prose',
    ]) {
      const rebuilt = parseInline(text)
        .map(s => (s.kind === 'bold' ? `**${s.value}**` : s.kind === 'code' ? `\`${s.value}\`` : s.value))
        .join('')
      expect(rebuilt).toBe(text)
    }
  })
})
