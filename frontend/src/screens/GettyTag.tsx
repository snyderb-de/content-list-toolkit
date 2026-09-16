import { useEffect, useState } from 'react'
import {
  CheckGettyReachability,
  CheckGettyTags,
  GetGettyDefaults,
  OpenPath,
  PickSheet,
  SaveGettyTagEdits,
  SaveGettyVocabulary,
} from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import Toggle from '../components/Toggle'

type Phase = 'idle' | 'checking' | 'done' | 'error'
type Source = 'live' | 'file' | 'none'

// A finding reads as one of three things, matching the written report: already
// fixed, needs correcting, or a judgement call. Anything the vocabulary could
// not confirm is an error rather than commentary — it is the substantive
// finding on this screen.
type FindingTone = 'fixed' | 'error' | 'review'

function findingTone(issue: main.TagIssue): FindingTone {
  if (issue.repaired) return 'fixed'
  if (issue.kind === 'unknown-term') return 'error'
  if (issue.kind === 'damaged-text') return 'error'
  if (issue.severity === 'blocks-upload') return 'error'
  return 'review'
}

// The kind is a machine token; the badge says it the way a person would.
const FINDING_LABELS: Record<string, string> = {
  'ghost-characters': 'invisible characters',
  'whitespace': 'spacing',
  'separator': 'separator',
  'tag-count': 'tag count',
  'empty-tag': 'empty tag',
  'duplicate-tag': 'duplicate',
  'damaged-text': 'damaged text',
  'field-limit': 'too long',
  'unknown-term': 'not in AAT',
  'term-case': 'spelling case',
  'not-checked': 'not checked',
}

function findingLabel(issue: main.TagIssue): string {
  return FINDING_LABELS[issue.kind] ?? issue.kind
}

// The prefix states what the reader has to do about it, before the detail
// explains what it is.
const TONE_PREFIX: Record<FindingTone, string> = {
  fixed: 'Fixed:',
  error: 'Must Fix:',
  review: 'Review:',
}

function rowStatus(row: main.TagRow): { text: string; tone: FindingTone } | null {
  const issues = row.result.issues ?? []
  if (issues.some((i) => findingTone(i) === 'error')) {
    return { text: 'Must Fix', tone: 'error' }
  }
  if (issues.some((i) => !i.repaired)) {
    return { text: 'Review', tone: 'review' }
  }
  return { text: 'Fixed', tone: 'fixed' }
}

export default function GettyTag() {
  const [sheetPath, setSheetPath] = useState('')
  const [source, setSource] = useState<Source>('live')
  const [vocabPath, setVocabPath] = useState('')
  const [writeCleaned, setWriteCleaned] = useState(true)
  const [writeReport, setWriteReport] = useState(true)

  const [reach, setReach] = useState<main.GettyReachability | null>(null)
  const [probing, setProbing] = useState(false)

  const [phase, setPhase] = useState<Phase>('idle')
  const [result, setResult] = useState<main.GettyCheckResult | null>(null)
  const [err, setErr] = useState('')

  // Edits are held by row until saved, so nothing is written to disk while
  // someone is still deciding.
  const [edits, setEdits] = useState<Record<number, string>>({})
  const [saving, setSaving] = useState(false)
  const [savedPath, setSavedPath] = useState('')

  useEffect(() => {
    GetGettyDefaults()
      .then((d) => {
        if (d.source) setSource(d.source as Source)
        if (d.vocabularyPath) setVocabPath(d.vocabularyPath)
      })
      .catch(() => {})
  }, [])

  // Probe on open and whenever the live source is selected, so the indicator
  // reflects this machine rather than an assumption about the network.
  useEffect(() => {
    if (source !== 'live') return
    setProbing(true)
    CheckGettyReachability()
      .then(setReach)
      .catch(() => setReach(null))
      .finally(() => setProbing(false))
  }, [source])

  const chooseSheet = async () => {
    const p = await PickSheet('Choose the exported Access sheet')
    if (p) setSheetPath(p)
  }

  const chooseVocabulary = async () => {
    const p = await PickSheet('Choose the AAT term list')
    if (p) {
      setVocabPath(p)
      SaveGettyVocabulary('file', p).catch(() => {})
    }
  }

  const pickSource = (next: Source) => {
    setSource(next)
    SaveGettyVocabulary(next, vocabPath).catch(() => {})
  }

  const run = async () => {
    if (!sheetPath) return
    setPhase('checking')
    setErr('')
    try {
      const r = await CheckGettyTags({
        sheetPath,
        source,
        vocabularyPath: vocabPath,
        writeCleaned,
        writeReport,
      } as main.GettyCheckOptions)
      setResult(r)
      setEdits({})
      setSavedPath('')
      setPhase('done')
    } catch (e: any) {
      setErr(String(e))
      setPhase('error')
    }
  }

  const reset = () => {
    setPhase('idle')
    setResult(null)
    setErr('')
    setEdits({})
    setSavedPath('')
  }

  const editRow = (row: number, tags: string) =>
    setEdits((current) => ({ ...current, [row]: tags }))

  // A suggestion replaces only the term it belongs to, leaving the rest of the
  // cell alone — the other tags in the row are usually fine.
  const applySuggestion = (row: number, current: string, term: string, replacement: string) => {
    const next = current
      .split(';')
      .map((t) => t.trim())
      .filter((t) => t !== '')
      .map((t) => (t.toLowerCase() === term.toLowerCase() ? replacement : t))
      .join('; ')
    editRow(row, next)
  }

  const save = async () => {
    if (!result) return
    const pending = Object.entries(edits).map(([row, tags]) => ({
      row: Number(row),
      tags,
    })) as main.GettyTagEdit[]
    if (pending.length === 0) return

    setSaving(true)
    setErr('')
    try {
      const saved = await SaveGettyTagEdits(
        {
          sheetPath,
          source,
          vocabularyPath: vocabPath,
          writeCleaned: true,
          writeReport,
        } as main.GettyCheckOptions,
        pending,
      )
      setResult({ ...result, report: saved.report, cleanedPath: saved.cleanedPath } as main.GettyCheckResult)
      setSavedPath(saved.cleanedPath)
      setEdits({})
    } catch (e: any) {
      setErr(String(e))
    } finally {
      setSaving(false)
    }
  }

  // ── Form ──────────────────────────────────────────────────
  if (phase === 'idle') {
    const liveBlocked = source === 'live' && reach !== null && !reach.reachable
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Getty Tag Check</h2>
          <p className="screen-subtitle">
            Clean the invisible characters that stop a CONTENTdm upload, and check every term
            against the Getty Art &amp; Architecture Thesaurus.
          </p>
        </div>

        <div className="card">
          <p className="card-title">Exported sheet</p>
          <div className="field">
            <label className="field-label">Exported Sheet</label>
            <div className="field-row">
              <input
                className="text-input monospace"
                value={sheetPath}
                onChange={(e) => setSheetPath(e.target.value)}
                placeholder="Access Main table exported to .xlsx, .csv, or tab-delimited .txt"
              />
              <button className="btn btn-outline btn-sm" onClick={chooseSheet}>Browse</button>
            </div>
          </div>
          <div className="info-text" style={{ marginTop: 12 }}>
            The check reads the <code>Tags</code> column. The original file is never modified —
            a corrected copy is written beside it.
          </div>
        </div>

        <div className="card">
          <p className="card-title">Check terms against</p>

          <div className="field">
            <label className="field-label">Verify Terms Against</label>
            <select
              className="select"
              value={source}
              onChange={(e) => pickSource(e.target.value as Source)}
            >
              <option value="live">Getty, live — authoritative</option>
              <option value="file">A term list on this machine — works offline</option>
              <option value="none">Do not verify terms — only clean the text</option>
            </select>
          </div>

          {source === 'live' && (
            <div className="info-text">
              {probing && 'Checking whether Getty is reachable from this machine…'}
              {!probing && reach !== null && (
                <span className={reach.reachable ? 'success-text' : 'danger-text'}>
                  {reach.reachable ? `● Getty is reachable — responded in ${reach.latencyMs} ms` : '● Getty is not reachable'}
                </span>
              )}
            </div>
          )}

          {source === 'file' && (
            <div className="field">
              <label className="field-label">Term List</label>
              <div className="field-row">
                <input
                  className="text-input monospace"
                  value={vocabPath}
                  onChange={(e) => setVocabPath(e.target.value)}
                  placeholder="AAT term list, one term per line"
                />
                <button className="btn btn-outline btn-sm" onClick={chooseVocabulary}>Browse</button>
              </div>
            </div>
          )}

          {liveBlocked && (
            <div className="info-text danger-text" style={{ marginTop: 12 }}>
              {reach?.detail} Terms cannot be verified from this machine right now. Use a term
              list, or clean the text only.
            </div>
          )}
        </div>

        <div className="card">
          <p className="card-title">Output</p>
          <Toggle label="Write a cleaned copy of the sheet" checked={writeCleaned} onChange={setWriteCleaned} />
          <Toggle label="Write a report of everything found" checked={writeReport} onChange={setWriteReport} />

          <button
            className="btn btn-primary btn-lg"
            style={{ marginTop: 16 }}
            onClick={run}
            disabled={!sheetPath || (source === 'file' && !vocabPath)}
          >
            Check Tags
          </button>
        </div>
      </div>
    )
  }

  // ── Checking ──────────────────────────────────────────────
  if (phase === 'checking') {
    return (
      <div>
        <div className="screen-header"><h2 className="screen-title">Checking…</h2></div>
        <div className="card">
          <div className="phase-badge">
            <span className="phase-dot" />
            {source === 'live'
              ? 'Cleaning the Tags column and asking Getty about each term'
              : 'Cleaning the Tags column'}…
          </div>
          <div className="stat-row">
            <span className="stat-row-label">Sheet</span>
            <span className="stat-row-value">{sheetPath}</span>
          </div>
        </div>
      </div>
    )
  }

  // ── Error ─────────────────────────────────────────────────
  if (phase === 'error') {
    return (
      <div>
        <div className="screen-header"><h2 className="screen-title">Check Failed</h2></div>
        <div className="card">
          <p className="danger-text" style={{ marginBottom: 16 }}>{err}</p>
          <button className="btn btn-outline" onClick={reset}>Try Again</button>
        </div>
      </div>
    )
  }

  // ── Done ──────────────────────────────────────────────────
  if (!result) return null
  const report = result.report
  const rows = report.rows ?? []
  const blocking = rows.filter((r) => r.result.issues?.some((i) => i.severity === 'blocks-upload')).length
  const needsReview = rows.filter((r) => r.result.issues?.some((i) => !i.repaired)).length
  const repaired = rows.filter((r) => r.result.cleaned !== r.result.original).length
  const cleanedPath = result.cleanedPath ?? ''
  const reportPath = result.reportPath ?? ''

  return (
    <div>
      <div className="screen-header">
        <h2 className="screen-title">Check Complete</h2>
        <p className={`screen-subtitle ${rows.length === 0 ? 'success-text' : ''}`}>
          {rows.length === 0
            ? `All ${report.totalRows} rows are ready to upload.`
            : `${rows.length} of ${report.totalRows} rows need attention.`}
        </p>
      </div>

      <div className="card">
        <p className="card-title">Results</p>
        <div className="stat-grid" style={{ marginBottom: 16 }}>
          <div className="stat-block">
            <div className="stat-block-label">Would have failed upload</div>
            <div className={`stat-block-value ${blocking > 0 ? 'danger-text' : 'success-text'}`}>{blocking}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Repaired</div>
            <div className="stat-block-value success-text">{repaired}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Need review</div>
            <div className="stat-block-value">{needsReview}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Elapsed</div>
            <div className="stat-block-value">{result.elapsed}</div>
          </div>
        </div>

        <div className="stat-row">
          <span className="stat-row-label">Tags column</span>
          <span className="stat-row-value">{report.columnLetter || `index ${report.columnIndex}`}</span>
        </div>
        <div className="stat-row">
          <span className="stat-row-label">Vocabulary</span>
          <span className="stat-row-value">{report.vocabularySource || 'not checked — structure only'}</span>
        </div>
        <div className="stat-row">
          <span className="stat-row-label">Empty Tags cells</span>
          <span className="stat-row-value">{report.emptyCells}</span>
        </div>

        <div className="result-actions">
          {cleanedPath && (
            <button className="btn btn-primary" onClick={() => OpenPath(cleanedPath)}>
              Open Cleaned Sheet
            </button>
          )}
          {reportPath && (
            <button className="btn btn-outline" onClick={() => OpenPath(reportPath)}>
              Open Report
            </button>
          )}
          <button className="btn btn-ghost" onClick={reset}>Check Another</button>
        </div>
      </div>

      {rows.length > 0 && (
        <div className="card">
          <div className="findings-header">
            <p className="card-title" style={{ margin: 0 }}>Findings by row</p>
            <div className="findings-actions">
              {savedPath && Object.keys(edits).length === 0 && (
                <span className="success-text">Saved to {savedPath.split('/').pop()}</span>
              )}
              {Object.keys(edits).length > 0 && (
                <span className="info-text">
                  {Object.keys(edits).length} row{Object.keys(edits).length === 1 ? '' : 's'} edited
                </span>
              )}
              <button
                className="btn btn-primary btn-sm"
                onClick={save}
                disabled={saving || Object.keys(edits).length === 0}
              >
                {saving ? 'Saving…' : 'Save to Cleaned Copy'}
              </button>
            </div>
          </div>
          <div className="info-text">
            Edits are written to the cleaned copy beside the sheet. The original export is
            never modified.
          </div>
          <div className="diff-table-wrap">
            <table className="diff-table getty-findings">
              <thead>
                <tr>
                  <th style={{ width: 56 }}>Row</th>
                  <th style={{ width: '38%' }}>Tags</th>
                  <th>Findings</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const status = rowStatus(row)
                  const changed = row.result.cleaned !== row.result.original
                  return (
                    <tr key={row.number}>
                      <td>
                        {row.number}
                        {status && (
                          <span className={`getty-row-status finding-${status.tone}`}>
                            <span className="finding-badge">{status.text}</span>
                          </span>
                        )}
                      </td>
                      <td>
                        <div className="current-file tag-before">{row.result.original}</div>
                        <input
                          className="text-input monospace"
                          value={edits[row.number] ?? row.result.cleaned}
                          onChange={(e) => editRow(row.number, e.target.value)}
                          spellCheck={false}
                        />
                      </td>
                      <td>
                        {(row.result.issues ?? []).map((issue, i) => (
                          <div key={i} className={`finding finding-${findingTone(issue)}`}>
                            <span className="finding-prefix">{TONE_PREFIX[findingTone(issue)]}</span>
                            <span className="finding-detail">
                              {findingLabel(issue)} — {issue.detail}
                            </span>
                          </div>
                        ))}
                        {(row.result.terms ?? [])
                          .filter((term) => term.checked && !term.found && (term.suggestions?.length ?? 0) > 0)
                          .map((term) => (
                            <div key={term.term} className="suggestion-row">
                              <span className="suggestion-label">Replace “{term.term}” with</span>
                              {(term.suggestions ?? []).map((s) => (
                                <button
                                  key={s}
                                  className="suggestion-chip"
                                  onClick={() =>
                                    applySuggestion(
                                      row.number,
                                      edits[row.number] ?? row.result.cleaned,
                                      term.term,
                                      s,
                                    )
                                  }
                                >
                                  {s}
                                </button>
                              ))}
                            </div>
                          ))}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
          <div className="info-text" style={{ marginTop: 12 }}>
            <span className="success-text">Fixed</span> — already corrected in the cleaned copy ·{' '}
            <span className="danger-text">Must Fix</span> — correct it before uploading ·{' '}
            Review — a judgement call.
          </div>
        </div>
      )}

      {report.vocabularySource && (
        <div className="info-text" style={{ marginTop: 16 }}>
          Term verification uses the Getty Art &amp; Architecture Thesaurus (AAT), J. Paul Getty
          Trust, under the Open Data Commons Attribution License (ODC-By) 1.0.
        </div>
      )}
    </div>
  )
}
