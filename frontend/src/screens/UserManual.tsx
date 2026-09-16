import React from 'react'
import manual from '../manual.json'

// The manual is content, not markup. It lives in manual.json so the screen and
// the HTML mirror in project-dashboard/ render the same words — a manual kept
// in two hand-written copies drifts, and the Getty section proved it by
// landing in one of them.
//
// Inline emphasis uses a deliberately tiny markup: **bold** and `code`. Both
// renderers parse it, so neither has to inject raw HTML and escaping stays
// correct on both sides.

type Block =
  | { type: 'paragraph'; text: string }
  | { type: 'steps'; items: string[] }
  | { type: 'list'; items: string[] }
  | { type: 'callout'; text: string; variant?: string }
  | { type: 'table'; headers: string[]; rows: string[][] }
  | { type: 'definitions'; items: { term: string; detail: string }[] }
  | { type: 'chips'; label: string; items: string[] }
  | { type: 'support'; items: { title: string; text: string }[] }

interface Section {
  id: string
  kicker: string
  title: string
  blocks: Block[]
}

const INLINE = /(\*\*[^*]+\*\*|`[^`]+`)/g

// Renders **bold** and `code` as elements rather than as HTML, so a stray
// angle bracket in the content can never become markup.
function inline(text: string): React.ReactNode[] {
  return text.split(INLINE).filter(Boolean).map((part, i) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={i}>{part.slice(2, -2)}</strong>
    }
    if (part.startsWith('`') && part.endsWith('`')) {
      return <code key={i}>{part.slice(1, -1)}</code>
    }
    return <React.Fragment key={i}>{part}</React.Fragment>
  })
}

function renderBlock(block: Block, key: number) {
  switch (block.type) {
    case 'paragraph':
      return <p key={key}>{inline(block.text)}</p>

    case 'steps':
      return (
        <ol key={key} className="manual-steps">
          {block.items.map((item, i) => <li key={i}>{inline(item)}</li>)}
        </ol>
      )

    case 'list':
      return (
        <ul key={key} className="manual-list">
          {block.items.map((item, i) => <li key={i}>{inline(item)}</li>)}
        </ul>
      )

    case 'callout':
      return (
        <div key={key} className={`manual-callout${block.variant === 'warning' ? ' manual-warning' : ''}`}>
          {inline(block.text)}
        </div>
      )

    case 'table':
      return (
        <table key={key} className="manual-table">
          <thead>
            <tr>{block.headers.map(h => <th key={h}>{h}</th>)}</tr>
          </thead>
          <tbody>
            {block.rows.map((row, i) => (
              <tr key={i}>{row.map((cell, j) => <td key={j}>{inline(cell)}</td>)}</tr>
            ))}
          </tbody>
        </table>
      )

    case 'definitions':
      return (
        <dl key={key} className="manual-definitions">
          {block.items.map(item => (
            <div key={item.term}>
              <dt>{item.term}</dt>
              <dd>{inline(item.detail)}</dd>
            </div>
          ))}
        </dl>
      )

    case 'chips':
      return (
        <ul key={key} className="manual-chip-list" aria-label={block.label}>
          {block.items.map(item => <li key={item}><code>{item}</code></li>)}
        </ul>
      )

    case 'support':
      return (
        <div key={key} className="manual-support-grid">
          {block.items.map(item => (
            <div key={item.title}>
              <h4>{item.title}</h4>
              <p>{inline(item.text)}</p>
            </div>
          ))}
        </div>
      )
  }
}

export default function UserManual() {
  const sections = manual.sections as Section[]

  return (
    <div className="manual-page">
      <div className="screen-header">
        <h2 className="screen-title">{manual.title}</h2>
        <p className="screen-subtitle">{manual.subtitle}</p>
      </div>

      <div className="manual-layout">
        <nav className="manual-toc" aria-label="User manual contents">
          {sections.map(section => (
            <a key={section.id} href={`#${section.id}`}>{section.title}</a>
          ))}
        </nav>

        <article className="manual-document" aria-label="Content List Toolkit user manual">
          {sections.map(section => (
            <section key={section.id} id={section.id} className="manual-section">
              <p className="manual-kicker">{section.kicker}</p>
              <h3>{section.title}</h3>
              {section.blocks.map((block, i) => renderBlock(block, i))}
            </section>
          ))}
        </article>
      </div>
    </div>
  )
}
