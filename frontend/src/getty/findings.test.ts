import { describe, expect, it } from 'vitest'
import type { main } from '../../wailsjs/go/models'
import {
  TONE_PREFIX,
  findingLabel,
  findingTone,
  replaceTermInCell,
  rowStatus,
} from './findings'

const issue = (kind: string, overrides: Partial<main.TagIssue> = {}): main.TagIssue =>
  ({ kind, severity: 'review', repaired: false, detail: '', ...overrides }) as main.TagIssue

const row = (issues: main.TagIssue[]): main.TagRow =>
  ({ number: 2, result: { issues } }) as unknown as main.TagRow

describe('findingTone', () => {
  it('treats anything already corrected as fixed', () => {
    expect(findingTone(issue('ghost-characters', { severity: 'blocks-upload', repaired: true }))).toBe('fixed')
  })

  it('treats a term the vocabulary rejected as an error, not a judgement call', () => {
    // It is the substantive finding on the screen. Filing it beside the
    // judgement calls is how it gets skimmed past.
    expect(findingTone(issue('unknown-term'))).toBe('error')
  })

  it('treats damaged text as an error', () => {
    expect(findingTone(issue('damaged-text'))).toBe('error')
  })

  it('treats an unrepaired upload blocker as an error', () => {
    expect(findingTone(issue('ghost-characters', { severity: 'blocks-upload' }))).toBe('error')
  })

  it('leaves everything else as a judgement call', () => {
    for (const kind of ['tag-count', 'duplicate-tag', 'term-case', 'field-limit', 'unbalanced-brackets']) {
      expect(findingTone(issue(kind))).toBe('review')
    }
  })
})

describe('findingLabel', () => {
  it('says the kind the way a person would', () => {
    expect(findingLabel(issue('ghost-characters'))).toBe('invisible characters')
    expect(findingLabel(issue('unknown-term'))).toBe('not in AAT')
    expect(findingLabel(issue('unbalanced-brackets'))).toBe('unclosed bracket')
  })

  it('falls back to the raw kind rather than showing nothing', () => {
    // A kind added in Go and not yet here must still render something.
    expect(findingLabel(issue('some-future-kind'))).toBe('some-future-kind')
  })
})

describe('rowStatus', () => {
  it('reports the worst finding in the row', () => {
    expect(rowStatus(row([issue('tag-count'), issue('unknown-term')]))?.text).toBe('Must Fix')
  })

  it('reports Review when nothing is an error but something remains', () => {
    expect(rowStatus(row([issue('tag-count')]))?.text).toBe('Review')
  })

  it('reports Fixed when cleaning resolved everything', () => {
    const status = rowStatus(row([issue('ghost-characters', { severity: 'blocks-upload', repaired: true })]))
    expect(status?.text).toBe('Fixed')
    expect(status?.tone).toBe('fixed')
  })
})

describe('TONE_PREFIX', () => {
  it('states what the reader has to do', () => {
    expect(TONE_PREFIX.fixed).toBe('Fixed:')
    expect(TONE_PREFIX.error).toBe('Must Fix:')
    expect(TONE_PREFIX.review).toBe('Review:')
  })
})

describe('replaceTermInCell', () => {
  it('replaces only the term it was given', () => {
    expect(
      replaceTermInCell('automobiles; counters (furniture; counter stools', 'counters (furniture', 'counters (furniture)'),
    ).toBe('automobiles; counters (furniture); counter stools')
  })

  it('leaves the other tags untouched', () => {
    // Rewriting the whole cell would discard corrections already made to it.
    expect(replaceTermInCell('a; b; c', 'b', 'B')).toBe('a; B; c')
  })

  it('matches the term without regard to case', () => {
    expect(replaceTermInCell('Landscapes; city plans', 'landscapes', 'coastal landscapes'))
      .toBe('coastal landscapes; city plans')
  })

  it('replaces every occurrence of a repeated term', () => {
    expect(replaceTermInCell('a; b; a', 'a', 'z')).toBe('z; b; z')
  })

  it('normalises the separator while it is there', () => {
    expect(replaceTermInCell('a;b ;  c', 'b', 'B')).toBe('a; B; c')
  })

  it('drops empty entries rather than leaving stray semicolons', () => {
    expect(replaceTermInCell('a;; b;', 'a', 'A')).toBe('A; b')
  })

  it('leaves a cell alone when the term is not in it', () => {
    expect(replaceTermInCell('a; b', 'zzz', 'Z')).toBe('a; b')
  })
})
