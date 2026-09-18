import { useEffect, useState } from 'react'
import {
  BuildAccessFileList,
  CheckGettyReachability,
  CheckGettyTags,
  GetBuiltinVocabularyInfo,
  GetGettyDefaults,
  ImportGettyVocabulary,
  OpenPath,
  PickFolder,
  PickGettyArchive,
  PickSheet,
  RecheckGettyReachability,
  RevealPath,
  SaveGettyTagEdits,
  SaveGettyVocabulary,
} from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import {
  FindingTone,
  TONE_PREFIX,
  findingLabel,
  findingTone,
  replaceTermInCell,
  rowStatus,
} from '../getty/findings'
import Toggle from '../components/Toggle'

type Phase = 'idle' | 'checking' | 'done' | 'error'
type Source = 'live' | 'builtin' | 'file' | 'none'

export default function GettyTag() {
  const [builtin, setBuiltin] = useState<main.BuiltinVocabularyInfo | null>(null)
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
  const [pass, setPass] = useState(0)
  const [importing, setImporting] = useState(false)
  const [importNote, setImportNote] = useState('')

  const [fileList, setFileList] = useState<main.AccessFileListResult | null>(null)
  const [buildingList, setBuildingList] = useState(false)
  const [listErr, setListErr] = useState('')

  useEffect(() => {
    GetGettyDefaults()
      .then((d) => {
        if (d.source) setSource(d.source as Source)
        if (d.vocabularyPath) setVocabPath(d.vocabularyPath)
        if (d.lastSheet) setSheetPath(d.lastSheet)
      })
      .catch(() => {})
  }, [])

  // The count and the date belong to the build, not to this screen: both
  // move when the bundled list is rebuilt, and a sentence here claiming
  // otherwise went stale once already.
  useEffect(() => {
    GetBuiltinVocabularyInfo().then(setBuiltin).catch(() => setBuiltin(null))
  }, [])

  // Probe once, the first time the live source is actually selected. The
  // answer is remembered in Go for the session, so returning to this screen or
  // flipping the dropdown costs no request — a red indicator must not turn
  // into a stream of calls at a host that is already refusing them.
  useEffect(() => {
    if (source !== 'live' || reach !== null) return
    setProbing(true)
    CheckGettyReachability()
      .then(setReach)
      .catch(() => setReach(null))
      .finally(() => setProbing(false))
  }, [source, reach])

  // Asking again is deliberate: the network changed, and the person at the
  // screen is the one who knows it.
  const recheckReachability = () => {
    setProbing(true)
    RecheckGettyReachability()
      .then(setReach)
      .catch(() => setReach(null))
      .finally(() => setProbing(false))
  }

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

  // Comes before the tag check in the workflow: the file names have to be in
  // Access before there is anything to export and check.
  const buildFileList = async () => {
    const dir = await PickFolder('Choose the folder holding the images')
    if (!dir) return
    setBuildingList(true)
    setListErr('')
    setFileList(null)
    try {
      setFileList(await BuildAccessFileList(dir))
    } catch (e: any) {
      setListErr(String(e))
    } finally {
      setBuildingList(false)
    }
  }

  // The app does not fetch the archive: Getty serves it over plain HTTP only,
  // and this app makes no unencrypted connections. So the archive is obtained
  // separately and converted here, which is also why the term list is written
  // beside it rather than into a folder chosen in a second dialog.
  const importList = async () => {
    const archive = await PickGettyArchive('Choose the Getty vocabulary archive (.zip)')
    if (!archive) return
    setImporting(true)
    setImportNote('')
    setErr('')
    try {
      const r = await ImportGettyVocabulary(archive)
      setVocabPath(r.path)
      setSource('file')
      SaveGettyVocabulary('file', r.path).catch(() => {})
      setImportNote(
        `${r.terms.toLocaleString()} English terms from ${r.archive}, published ${r.published}.`,
      )
    } catch (e: any) {
      setErr(String(e))
    } finally {
      setImporting(false)
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
      setPass(0)
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
    setPass(0)
  }

  const editRow = (row: number, tags: string) =>
    setEdits((current) => ({ ...current, [row]: tags }))

  // A suggestion replaces only the term it belongs to, leaving the rest of the
  // cell alone — the other tags in the row are usually fine.
  const applySuggestion = (row: number, current: string, term: string, replacement: string) =>
    editRow(row, replaceTermInCell(current, term, replacement))

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
      // Every edit made so far is re-sent, not just the newest. Each save
      // rebuilds the cleaned copy from the untouched source, so sending only
      // the latest batch would silently drop every earlier correction.
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
      setResult({
        ...result,
        report: saved.report,
        cleanedPath: saved.cleanedPath,
      } as main.GettyCheckResult)
      setSavedPath(saved.cleanedPath)
      setPass((p) => p + 1)
    } catch (e: any) {
      setErr(String(e))
    } finally {
      setSaving(false)
    }
  }

  // Re-check without saving, for when the sheet was corrected in Excel while
  // this screen was open.
  const recheck = async () => {
    setSaving(true)
    setErr('')
    try {
      const r = await CheckGettyTags({
        sheetPath,
        source,
        vocabularyPath: vocabPath,
        writeCleaned: true,
        writeReport,
      } as main.GettyCheckOptions)
      setResult(r)
      setEdits({})
      setPass((p) => p + 1)
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
          <p className="card-title">Step 1 · Access file list</p>
          <p className="info-text" style={{ marginBottom: 12 }}>
            Builds the <code>Item Number</code> and <code>File Name (Cdm)</code> columns from a
            folder of images, ready to paste into Access. Replaces the Command Prompt{' '}
            <code>dir /b</code> step, and fills both columns rather than one.
          </p>
          <button className="btn btn-outline" onClick={buildFileList} disabled={buildingList}>
            {buildingList ? 'Reading folder…' : 'Build File List'}
          </button>
          {listErr && <p className="danger-text" style={{ marginTop: 10 }}>{listErr}</p>}
          {fileList && (
            <>
              <div className="stat-row" style={{ marginTop: 12 }}>
                <span className="stat-row-label">Files listed</span>
                <span className="stat-row-value success-text">{fileList.files.toLocaleString()}</span>
              </div>
              {fileList.skipped > 0 && (
                <div className="stat-row">
                  <span className="stat-row-label">Skipped</span>
                  <span className="stat-row-value">
                    {fileList.skipped} (folders, hidden and system files)
                  </span>
                </div>
              )}
              <div className="stat-row">
                <span className="stat-row-label">First</span>
                <span className="stat-row-value">{fileList.first}</span>
              </div>
              <div className="stat-row">
                <span className="stat-row-label">Last</span>
                <span className="stat-row-value">{fileList.last}</span>
              </div>
              <div className="result-actions">
                <button className="btn btn-primary btn-sm" onClick={() => OpenPath(fileList.path)}>
                  Open File List
                </button>
                <button className="btn btn-outline btn-sm" onClick={() => RevealPath(fileList.path)}>
                  Show in Folder
                </button>
              </div>
            </>
          )}
        </div>

        <div className="card">
          <p className="card-title">Step 2 · Exported sheet</p>
          <div className="field">
            {/* The card title already says what this is; a visible label here
                repeated it. The accessible name stays on the input. */}
            <div className="field-row">
              <input
                className="text-input monospace"
                aria-label="Exported sheet"
                value={sheetPath}
                onChange={(e) => setSheetPath(e.target.value)}
              />
              <button className="btn btn-outline btn-sm" onClick={chooseSheet}>Browse</button>
            </div>
          </div>
          <div className="info-text" style={{ marginTop: 12 }}>
            Accepts <code>.xlsx</code>, <code>.csv</code>, and tab-delimited <code>.txt</code>. The
            check reads the <code>Tags</code> column. The original file is never modified — a
            corrected copy is written beside it.
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
              <option value="builtin">Built-in list — works offline, no setup</option>
              <option value="file">A term list on this machine</option>
              <option value="none">Do not verify terms — only clean the text</option>
            </select>
          </div>

          {source === 'live' && (
            <div className="info-text">
              {probing && 'Checking whether Getty is reachable from this machine…'}
              {!probing && reach !== null && (
                <>
                  <span className={reach.reachable ? 'success-text' : 'danger-text'}>
                    {reach.reachable ? `● Getty is reachable — responded in ${reach.latencyMs} ms` : '● Getty is not reachable'}
                  </span>
                  {' · '}
                  Checked once at {new Date(reach.checkedAt).toLocaleTimeString()}.{' '}
                  <button className="btn btn-outline btn-sm" onClick={recheckReachability}>
                    Check again
                  </button>
                </>
              )}
            </div>
          )}

          {source === 'builtin' && (
            <div className="info-text">
              {builtin
                ? `${builtin.terms.toLocaleString()} English terms from Getty’s ${builtin.date} archive, `
                : 'The Getty term list '}
              shipped inside the application. It answers for the day that archive was made, and
              Getty keeps revising the thesaurus, so a term this list accepts may since have been
              renamed — the live check is the current authority.
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
              <button className="btn btn-outline btn-sm" onClick={importList} disabled={importing}>
                {importing ? 'Reading the archive…' : 'Build list from a Getty archive'}
              </button>
              <div className="info-text" style={{ marginTop: 8 }}>
                {importNote ||
                  'Converts a Getty relational archive (aat_rel_NNNN.zip) you already have into a term list, written beside the archive. The app does not download it: Getty serves those archives over an unencrypted connection, which this app will not make. The list takes the date of the archive you build it from, and Getty keeps revising the thesaurus, so a term it accepts may since have been renamed. Treat it as a fallback, not as the authority.'}
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
  const blocking = rows.filter((r) => (r.result.issues ?? []).some((i) => i.severity === 'blocks-upload')).length
  const needsReview = rows.filter((r) => (r.result.issues ?? []).some((i) => !i.repaired)).length
  const repaired = rows.filter((r) => r.result.cleaned !== r.result.original).length
  const cleanedPath = result.cleanedPath ?? ''
  const reportPath = result.reportPath ?? ''
  const editCount = Object.keys(edits).length

  return (
    <div>
      <div className="screen-header">
        <h2 className="screen-title">{pass > 0 ? `Re-checked (pass ${pass + 1})` : 'Check Complete'}</h2>
        <p className={`screen-subtitle ${rows.length === 0 ? 'success-text' : ''}`}>
          {rows.length === 0
            ? `Nothing left to fix — all ${report.totalRows} rows are ready to upload.`
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
        {report.vocabularyNote && (
          <div className="info-text danger-text" style={{ marginTop: 8 }}>
            {report.vocabularyNote}
          </div>
        )}
        <div className="stat-row">
          <span className="stat-row-label">Empty Tags cells</span>
          <span className="stat-row-value">{report.emptyCells}</span>
        </div>

        {err && <p className="danger-text" style={{ marginTop: 12 }}>{err}</p>}

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
          {/* Both outputs are written beside the source, so one reveal covers
              them; it selects whichever was actually written. */}
          {(cleanedPath || reportPath) && (
            <button className="btn btn-outline" onClick={() => RevealPath(cleanedPath || reportPath)}>
              Show in Folder
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
              {editCount > 0 && (
                <span className="info-text">
                  {editCount} row{editCount === 1 ? '' : 's'} edited
                </span>
              )}
              {editCount === 0 && savedPath && (
                <span className="success-text">Saved to {savedPath.split('/').pop()}</span>
              )}
              <button className="btn btn-outline btn-sm" onClick={recheck} disabled={saving}>
                {saving ? 'Working…' : 'Re-check'}
              </button>
              <button className="btn btn-primary btn-sm" onClick={save} disabled={saving || editCount === 0}>
                {saving ? 'Working…' : 'Apply Fixes & Re-check'}
              </button>
            </div>
          </div>
          <div className="info-text" style={{ marginBottom: 4 }}>
            Fix what you can here, then apply and re-check. Repeat until nothing is left.
            Edits are written to the cleaned copy beside the sheet — the original export is
            never modified.
          </div>

          <div className="diff-table-wrap">
            <table className="diff-table getty-findings">
              <thead>
                <tr>
                  <th style={{ width: 56 }}>Row</th>
                  <th style={{ width: '40%' }}>Tags</th>
                  <th>What to do</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => {
                  const status = rowStatus(row)
                  const current = edits[row.number] ?? row.result.cleaned
                  const issues = row.result.issues ?? []
                  // Term-level findings are grouped with their own replacements
                  // below, so they are not repeated in the row-level list.
                  const rowIssues = issues.filter(
                    (i) =>
                      i.kind !== 'unknown-term' &&
                      i.kind !== 'term-case' &&
                      i.kind !== 'variant-term',
                  )
                  const termFindings = (row.result.terms ?? []).filter(
                    (t) => t.checked && (!t.found || (t.preferredLabel && t.preferredLabel !== t.term)),
                  )

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
                          value={current}
                          onChange={(e) => editRow(row.number, e.target.value)}
                          spellCheck={false}
                        />
                      </td>
                      <td>
                        {rowIssues.map((issue, i) => (
                          <div key={i} className={`finding finding-${findingTone(issue)}`}>
                            <span className="finding-prefix">{TONE_PREFIX[findingTone(issue)]}</span>
                            <span className="finding-detail">
                              {findingLabel(issue)} — {issue.detail}
                            </span>
                          </div>
                        ))}

                        {termFindings.map((term) => {
                          const unknown = !term.found
                          const options = unknown
                            ? term.suggestions ?? []
                            : term.preferredLabel
                              ? [term.preferredLabel]
                              : []
                          // A variant has to be replaced, the same as an
                          // unknown term. The difference is that its
                          // replacement is known, so the chip below is the
                          // answer rather than a guess.
                          const mustFix = unknown || term.variant
                          return (
                            <div key={term.term} className="term-finding">
                              <div className={`finding ${mustFix ? 'finding-error' : 'finding-review'}`}>
                                <span className="finding-prefix">
                                  {mustFix ? 'Must Fix:' : 'Review:'}
                                </span>
                                <span className="finding-detail">
                                  <strong>{term.term}</strong>{' '}
                                  {unknown
                                    ? 'is not a term in the vocabulary'
                                    : term.variant
                                      ? `is a variant — the preferred term is “${term.preferredLabel}”`
                                      : `is spelled “${term.preferredLabel}” in the vocabulary`}
                                </span>
                              </div>
                              {options.length > 0 ? (
                                <div className="suggestion-row">
                                  <span className="suggestion-label">Replace with</span>
                                  {options.map((option) => (
                                    <button
                                      key={option}
                                      className="suggestion-chip"
                                      onClick={() => applySuggestion(row.number, current, term.term, option)}
                                    >
                                      {option}
                                    </button>
                                  ))}
                                </div>
                              ) : (
                                <div className="suggestion-row">
                                  <span className="suggestion-label">
                                    No near matches — edit the tag above directly.
                                  </span>
                                </div>
                              )}
                            </div>
                          )
                        })}
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
