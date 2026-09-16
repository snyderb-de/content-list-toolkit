import type { main } from '../../wailsjs/go/models'

// The decisions the Getty screen makes about a finding, kept apart from how it
// draws them. Classifying a finding and rewriting a tag are the parts that can
// be wrong in a way a person would not notice; rendering is the part that is
// obvious the moment you look at it.

// A finding reads as one of three things, matching the written report: already
// fixed, needs correcting, or a judgement call.
export type FindingTone = 'fixed' | 'error' | 'review'

export const TONE_PREFIX: Record<FindingTone, string> = {
  fixed: 'Fixed:',
  error: 'Must Fix:',
  review: 'Review:',
}

// Anything the vocabulary could not confirm is an error rather than
// commentary: it is the substantive finding on the screen, and filing it
// beside judgement calls is how it gets skimmed past.
export function findingTone(issue: main.TagIssue): FindingTone {
  if (issue.repaired) return 'fixed'
  if (issue.kind === 'unknown-term') return 'error'
  if (issue.kind === 'damaged-text') return 'error'
  if (issue.severity === 'blocks-upload') return 'error'
  return 'review'
}

// The kind is a machine token; the badge says it the way a person would.
const FINDING_LABELS: Record<string, string> = {
  'ghost-characters': 'invisible characters',
  whitespace: 'spacing',
  separator: 'separator',
  'tag-count': 'tag count',
  'empty-tag': 'empty tag',
  'duplicate-tag': 'duplicate',
  'damaged-text': 'damaged text',
  'field-limit': 'too long',
  'unknown-term': 'not in AAT',
  'term-case': 'spelling case',
  'not-checked': 'not checked',
  'unbalanced-brackets': 'unclosed bracket',
  'qualifier-unchecked': 'bracketed part unchecked',
}

export function findingLabel(issue: main.TagIssue): string {
  return FINDING_LABELS[issue.kind] ?? issue.kind
}

// One word for the whole row, so a long table can be scanned down one column.
// The worst finding in the row decides it.
export function rowStatus(row: main.TagRow): { text: string; tone: FindingTone } | null {
  const issues = row.result.issues ?? []
  if (issues.some(i => findingTone(i) === 'error')) return { text: 'Must Fix', tone: 'error' }
  if (issues.some(i => !i.repaired)) return { text: 'Review', tone: 'review' }
  return { text: 'Fixed', tone: 'fixed' }
}

// Replaces one term in a cell and leaves the rest alone.
//
// Accepting a suggestion should change the term it belongs to and nothing
// else: the other tags in the row are usually fine, and rewriting the whole
// cell would quietly discard corrections already made to it.
export function replaceTermInCell(cell: string, term: string, replacement: string): string {
  return cell
    .split(';')
    .map(t => t.trim())
    .filter(t => t !== '')
    .map(t => (t.toLowerCase() === term.toLowerCase() ? replacement : t))
    .join('; ')
}
