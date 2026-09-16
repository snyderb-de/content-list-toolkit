// The manual uses a deliberately small inline markup so its content can live
// in JSON and be rendered by both the app and the HTML mirror: **bold** and
// `code`.
//
// Parsing is separated from rendering so neither renderer has to inject raw
// HTML. The React side turns these segments into elements; the mirror escapes
// them before writing tags. Content can therefore never introduce markup of
// its own, whatever it contains.

export type InlineSegment =
  | { kind: 'text'; value: string }
  | { kind: 'bold'; value: string }
  | { kind: 'code'; value: string }

const PATTERN = /(\*\*[^*]+\*\*|`[^`]+`)/g

export function parseInline(text: string): InlineSegment[] {
  return text
    .split(PATTERN)
    .filter(part => part !== '')
    .map(part => {
      if (part.startsWith('**') && part.endsWith('**') && part.length > 4) {
        return { kind: 'bold', value: part.slice(2, -2) } as const
      }
      if (part.startsWith('`') && part.endsWith('`') && part.length > 2) {
        return { kind: 'code', value: part.slice(1, -1) } as const
      }
      return { kind: 'text', value: part } as const
    })
}
