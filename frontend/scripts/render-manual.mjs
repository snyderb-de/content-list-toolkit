// Renders the in-app user manual to a standalone HTML mirror.
//
// The manual is authored once in src/manual.json. The Getty section showed why:
// the manual existed as two hand-written copies, an edit landed in one of them,
// and nothing noticed. This script is run by `npm run build`, and
// scripts/dev_check.sh fails when the committed mirror is out of date, so the
// two cannot drift apart silently.
//
// Usage:
//   node scripts/render-manual.mjs           write the mirror
//   node scripts/render-manual.mjs --check   exit non-zero if it is stale

import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const manualPath = resolve(here, '../src/manual.json')
const outputPath = resolve(here, '../../project-dashboard/app-user-manual.html')

const manual = JSON.parse(readFileSync(manualPath, 'utf8'))

const escape = (s) =>
  String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')

// The same tiny markup the React renderer understands: **bold** and `code`.
// Escaping happens first, so content can never introduce markup of its own.
const inline = (text) =>
  escape(text)
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/`([^`]+)`/g, '<code>$1</code>')

const block = (b) => {
  switch (b.type) {
    case 'paragraph':
      return `<p>${inline(b.text)}</p>`
    case 'steps':
      return `<ol class="steps">${b.items.map((i) => `<li>${inline(i)}</li>`).join('')}</ol>`
    case 'list':
      return `<ul class="list">${b.items.map((i) => `<li>${inline(i)}</li>`).join('')}</ul>`
    case 'callout':
      return `<div class="callout${b.variant === 'warning' ? ' warning' : ''}">${inline(b.text)}</div>`
    case 'table':
      return `<div class="table-wrap"><table>
<thead><tr>${b.headers.map((h) => `<th>${escape(h)}</th>`).join('')}</tr></thead>
<tbody>${b.rows.map((r) => `<tr>${r.map((c) => `<td>${inline(c)}</td>`).join('')}</tr>`).join('')}</tbody>
</table></div>`
    case 'definitions':
      return `<dl class="definitions">${b.items
        .map((i) => `<div><dt>${escape(i.term)}</dt><dd>${inline(i.detail)}</dd></div>`)
        .join('')}</dl>`
    case 'chips':
      return `<ul class="chips" aria-label="${escape(b.label)}">${b.items
        .map((i) => `<li><code>${escape(i)}</code></li>`)
        .join('')}</ul>`
    case 'support':
      return `<div class="support">${b.items
        .map((i) => `<div><h4>${escape(i.title)}</h4><p>${inline(i.text)}</p></div>`)
        .join('')}</div>`
    default:
      throw new Error(`unknown block type: ${b.type}`)
  }
}

const section = (s) => `<section id="${escape(s.id)}">
<p class="kicker">${escape(s.kicker)}</p>
<h2>${escape(s.title)}</h2>
${s.blocks.map(block).join('\n')}
</section>`

const html = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>${escape(manual.title)} — Content List Toolkit</title>
<style>
:root {
  --ground: #ffffff; --surface: #f7f8fa; --ink: #1b2430; --ink-soft: #4a5663;
  --ink-faint: #6e7a87; --rule: #dde2e8; --accent: #1f5f8b; --warn: #8a5a17;
  --warn-wash: rgba(138, 90, 23, 0.09);
}
@media (prefers-color-scheme: dark) {
  :root {
    --ground: #10151b; --surface: #171e26; --ink: #e6ebf1; --ink-soft: #aeb9c4;
    --ink-faint: #8794a1; --rule: #2a3440; --accent: #6fb4e0; --warn: #d5a35c;
    --warn-wash: rgba(213, 163, 92, 0.12);
  }
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--ground); color: var(--ink);
  font: 16px/1.62 -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  -webkit-font-smoothing: antialiased;
}
.page { max-width: 54rem; margin: 0 auto; padding: 3.5rem 1.5rem 5rem; }
header { border-bottom: 1px solid var(--rule); padding-bottom: 1.5rem; margin-bottom: 2rem; }
h1 { font-size: 2rem; margin: 0 0 .4rem; letter-spacing: -.015em; }
.lede { color: var(--ink-soft); margin: 0; }
.mirror-note { margin-top: 1rem; font-size: .84rem; color: var(--ink-faint); }
nav { display: flex; flex-wrap: wrap; gap: .5rem 1rem; margin-bottom: 2.5rem; }
nav a { color: var(--accent); text-decoration: none; font-size: .9rem; }
nav a:hover, nav a:focus-visible { text-decoration: underline; }
section { margin-bottom: 3rem; scroll-margin-top: 1rem; }
.kicker {
  font-size: .7rem; letter-spacing: .1em; text-transform: uppercase;
  color: var(--ink-faint); margin: 0 0 .3rem;
}
h2 { font-size: 1.4rem; margin: 0 0 .9rem; letter-spacing: -.01em; }
h4 { font-size: .95rem; margin: 0 0 .25rem; }
p { color: var(--ink-soft); margin: 0 0 1rem; }
.steps, .list { color: var(--ink-soft); margin: 0 0 1rem; padding-left: 1.3rem; }
.steps li, .list li { margin-bottom: .35rem; }
.callout {
  border-left: 3px solid var(--accent); background: var(--surface);
  padding: .85rem 1.1rem; margin: 0 0 1rem; color: var(--ink-soft); border-radius: 0 3px 3px 0;
}
.callout.warning { border-left-color: var(--warn); background: var(--warn-wash); }
.table-wrap { overflow-x: auto; margin: 0 0 1rem; }
table { border-collapse: collapse; width: 100%; font-size: .92rem; min-width: 30rem; }
th {
  text-align: left; font-size: .72rem; letter-spacing: .07em; text-transform: uppercase;
  color: var(--ink-faint); padding: .6rem .8rem; border-bottom: 1px solid var(--rule);
}
td { padding: .65rem .8rem; border-bottom: 1px solid var(--rule); color: var(--ink-soft); vertical-align: top; }
.definitions { margin: 0 0 1rem; }
.definitions > div { display: grid; grid-template-columns: minmax(8rem, 12rem) 1fr; gap: 1rem; padding: .5rem 0; border-bottom: 1px solid var(--rule); }
dt { font-weight: 600; color: var(--ink); }
dd { margin: 0; color: var(--ink-soft); }
.chips { list-style: none; display: flex; flex-wrap: wrap; gap: .4rem; padding: 0; margin: 0 0 1rem; }
.chips li { background: var(--surface); border: 1px solid var(--rule); border-radius: 999px; padding: .15rem .6rem; }
.support { display: grid; grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr)); gap: 1.25rem; }
.support p { margin: 0; }
code {
  font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: .88em;
  background: var(--surface); border-radius: 3px; padding: .1em .35em; color: var(--ink);
}
footer { border-top: 1px solid var(--rule); padding-top: 1.25rem; font-size: .82rem; color: var(--ink-faint); }
</style>
</head>
<body>
<div class="page">
<header>
<h1>${escape(manual.title)}</h1>
<p class="lede">${escape(manual.subtitle)}</p>
<p class="mirror-note">This page mirrors the User Manual built into the application. Both are generated from the same source, so they always say the same thing.</p>
</header>

<nav aria-label="Contents">
${manual.sections.map((s) => `<a href="#${escape(s.id)}">${escape(s.title)}</a>`).join('\n')}
</nav>

${manual.sections.map(section).join('\n\n')}

<footer>Generated from <code>frontend/src/manual.json</code>. Edit that file, not this page.</footer>
</div>
</body>
</html>
`

const checking = process.argv.includes('--check')

if (checking) {
  if (!existsSync(outputPath)) {
    console.error('user manual mirror is missing; run: npm --prefix frontend run manual')
    process.exit(1)
  }
  if (readFileSync(outputPath, 'utf8') !== html) {
    console.error('user manual mirror is out of date; run: npm --prefix frontend run manual')
    process.exit(1)
  }
  console.log('user manual mirror is up to date')
} else {
  writeFileSync(outputPath, html)
  console.log(`wrote ${outputPath}`)
}
