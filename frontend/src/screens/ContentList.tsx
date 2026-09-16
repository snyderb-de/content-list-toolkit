import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { CheckOutputExists, GetScanDefaults, OpenPath, SaveSettings, StartScan, CancelScan, ValidateScanPaths } from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import FolderPicker from '../components/FolderPicker'
import ProgressBar from '../components/ProgressBar'
import Toggle from '../components/Toggle'
import { AgencyTemplateFields, HASH_ALGORITHMS, ScanDonePayload, ScanOptions, ScanProgressPayload } from '../types'

type Phase = 'idle' | 'scanning' | 'done' | 'error' | 'canceled' | 'confirm-overwrite'

function defaultOutputFilename(sourceDir: string): string {
  const parts = sourceDir.replace(/\\/g, '/').split('/').filter(Boolean)
  const name = parts.pop() || 'content-list'
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}-${pad(now.getMinutes())}-${pad(now.getSeconds())}`
  return `${name}-content-list-${stamp}.csv`
}

const DEFAULT_AGENCY_FIELDS: AgencyTemplateFields = {
  rg: '',
  sg: '',
  series: '',
  rcSeries: '',
  deptOrganization: '',
  division: '',
  section: '',
  unit: '',
  rcSeriesName: '',
  beginDate: '',
  endDate: '',
  description: '',
  location: '',
  materialType: 'Born Digital',
  comments: '',
  confidential: '',
  dispositionDate: '',
  boxNum: '',
  tdNum: '',
  locationId: '',
  recordLevel: 'Item',
}

const freshAgencyFields = (): AgencyTemplateFields => ({ ...DEFAULT_AGENCY_FIELDS })

const DEFAULT_OPTS: ScanOptions = {
  sourceDir: '', outputDir: '', outputFile: '',
  hashAlgorithm: 'blake3',
  excludeHidden: true, excludeSystem: true,
  createXLSX: true, preserveZeros: true, deleteCSV: true,
  excludedExts: '',
  foldersOnly: false,
  folderDepth: 0,
  agencyTemplate: false,
  agencyFields: freshAgencyFields(),
  releaseFolder: '',
}

const toWailsScanOptions = (value: ScanOptions): main.ScanOptions =>
  value as unknown as main.ScanOptions

function defaultFolderListFilename(sourceDir: string): string {
  const parts = sourceDir.replace(/\\/g, '/').split('/').filter(Boolean)
  const name = parts.pop() || 'folder-list'
  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}-${pad(now.getMinutes())}-${pad(now.getSeconds())}`
  return `${name}-folder-list-${stamp}.csv`
}

function fmtNum(n: number)  { return n.toLocaleString() }
function fmtBytes(b: number) {
  if (b < 1024) return `${b} B`
  if (b < 1024 ** 2) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 ** 3) return `${(b / 1024 ** 2).toFixed(1)} MB`
  if (b < 1024 ** 4) return `${(b / 1024 ** 3).toFixed(2)} GB`
  return `${(b / 1024 ** 4).toFixed(2)} TB`
}
function fmtSecs(s: number) {
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${(s % 60).toFixed(0)}s`
}
function fmtETA(etaSecs: number, phase: string) {
  if (phase !== 'Scanning') return phase === 'Counting' ? 'after count' : '…'
  if (etaSecs <= 0) return 'calculating…'
  return fmtSecs(etaSecs)
}

function agencyCodeError(fields: AgencyTemplateFields) {
  const checks: Array<[keyof AgencyTemplateFields, string, number]> = [
    ['rg', 'RG', 4],
    ['sg', 'SG', 3],
    ['series', 'Series', 3],
  ]
  for (const [key, label, width] of checks) {
    const value = fields[key].trim()
    if (!value) continue
    if (!/^\d+$/.test(value)) return `${label} must use digits only.`
    if (value.length > width) return `${label} must be ${width} digits or fewer.`
  }
  return ''
}

export default function ContentList() {
  const [opts, setOpts]       = useState<ScanOptions>(DEFAULT_OPTS)
  const [phase, setPhase]     = useState<Phase>('idle')
  const [progress, setProgress] = useState<ScanProgressPayload | null>(null)
  const [result, setResult]   = useState<ScanDonePayload | null>(null)
  const [err, setErr]         = useState('')
  const [pathError, setPathError] = useState('')

  useEffect(() => {
    if (opts.sourceDir) {
      setOpts(o => ({ ...o, outputFile: opts.foldersOnly ? defaultFolderListFilename(opts.sourceDir) : defaultOutputFilename(opts.sourceDir) }))
    }
  }, [opts.sourceDir])

  useEffect(() => {
    if (opts.sourceDir) {
      setOpts(o => ({ ...o, outputFile: opts.foldersOnly ? defaultFolderListFilename(opts.sourceDir) : defaultOutputFilename(opts.sourceDir) }))
    }
  }, [opts.foldersOnly])

  useEffect(() => {
    if (!opts.sourceDir && !opts.outputDir) return
    ValidateScanPaths(opts.sourceDir, opts.outputDir)
      .then(setPathError)
      .catch(() => {})
  }, [opts.sourceDir, opts.outputDir])

  useEffect(() => {
    GetScanDefaults()
      .then(d => setOpts(o => ({
        ...o,
        ...d,
        // Defaults arrive asynchronously and can land after a folder has
        // already been chosen. Spreading them wholesale would reset sourceDir
        // to the startup directory and blank the generated output filename —
        // GetScanDefaults sets SourceDir but never OutputFile — so a fast
        // click would silently scan the wrong folder. Anything already filled
        // in wins over the default.
        sourceDir: o.sourceDir || d.sourceDir,
        outputDir: o.outputDir || d.outputDir,
        outputFile: o.outputFile || d.outputFile,
        agencyFields: freshAgencyFields(),
      })))
      .catch(() => {})
    return () => {
      EventsOff('scan:progress')
      EventsOff('scan:done')
      EventsOff('scan:error')
      EventsOff('scan:canceled')
    }
  }, [])

  const set = (key: keyof ScanOptions, value: any) =>
    setOpts(o => ({ ...o, [key]: value }))

  const setAgency = (key: keyof AgencyTemplateFields, value: string) =>
    setOpts(o => ({ ...o, agencyFields: { ...DEFAULT_AGENCY_FIELDS, ...o.agencyFields, [key]: value } }))

  const doScan = async () => {
    setPhase('scanning')
    setProgress(null)
    setErr('')

    EventsOn('scan:progress', (d: ScanProgressPayload) => setProgress(d))
    EventsOn('scan:done',     (d: ScanDonePayload)     => { setResult(d); setPhase('done') })
    EventsOn('scan:error',    (msg: string)             => { setErr(msg); setPhase('error') })
    EventsOn('scan:canceled', ()                        => setPhase('canceled'))

    try {
      const apiOpts = toWailsScanOptions(opts)
      await StartScan(apiOpts)
      SaveSettings(toWailsScanOptions({ ...opts, agencyFields: freshAgencyFields() })).catch(() => {})
    } catch (e: any) {
      EventsOff('scan:progress'); EventsOff('scan:done')
      EventsOff('scan:error');    EventsOff('scan:canceled')
      setErr(String(e))
      setPhase('error')
    }
  }

  const start = async () => {
    const exists = await CheckOutputExists(toWailsScanOptions(opts)).catch(() => false)
    if (exists) {
      setPhase('confirm-overwrite')
    } else {
      await doScan()
    }
  }

  const cancel = () => CancelScan().catch(() => {})

  const reset = (clearAgencyFields = false) => {
    EventsOff('scan:progress'); EventsOff('scan:done')
    EventsOff('scan:error');    EventsOff('scan:canceled')
    setPhase('idle'); setProgress(null); setResult(null); setErr('')
    if (clearAgencyFields) {
      setOpts(o => ({ ...o, agencyFields: freshAgencyFields() }))
    }
  }

  const sameFolders = !!(opts.sourceDir && opts.outputDir && opts.sourceDir === opts.outputDir)
  const agencyMissingRCSeries = opts.agencyTemplate && !opts.agencyFields.rcSeries.trim()
  const agencyFieldError = opts.agencyTemplate ? agencyCodeError(opts.agencyFields) : ''
  const canStart = opts.sourceDir.length > 0 && opts.outputDir.length > 0 && !sameFolders && !pathError && !agencyMissingRCSeries && !agencyFieldError

  // ── Confirm overwrite ─────────────────────────────────────
  if (phase === 'confirm-overwrite') {
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">File Already Exists</h2>
          <p className="screen-subtitle">Output file will be overwritten.</p>
        </div>
        <div className="card">
          <p className="info-text" style={{ marginBottom: 16 }}>
            A scan output file already exists at the chosen location. Overwrite it?
          </p>
          <div className="result-actions">
            <button className="btn btn-danger" onClick={doScan}>Overwrite</button>
            <button className="btn btn-outline" onClick={() => setPhase('idle')}>Cancel</button>
          </div>
        </div>
      </div>
    )
  }

  // ── Form ──────────────────────────────────────────────────
  if (phase === 'idle' || phase === 'canceled') {
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Content List</h2>
          <p className="screen-subtitle">Recursively scan a folder and write a CSV inventory.</p>
        </div>

        {phase === 'canceled' && (
          <div className="card" style={{ marginBottom: 16, borderColor: 'var(--warning)' }}>
            <span className="danger-text">Scan was stopped.</span>
          </div>
        )}

        {/* Folders */}
        <div className="card">
          <p className="card-title">Folders</p>
          <FolderPicker label="Source Folder" value={opts.sourceDir} onChange={v => set('sourceDir', v)} />
          <FolderPicker label="Output Folder" value={opts.outputDir} onChange={v => set('outputDir', v)} />
          {sameFolders && (
            <p className="danger-text" style={{ margin: '4px 0 8px' }}>
              Source and output folders must be different.
            </p>
          )}
          {!sameFolders && pathError && (
            <p className="danger-text" style={{ margin: '4px 0 8px' }}>{pathError}</p>
          )}
          <div className="field">
            <label className="field-label">Output Filename</label>
            <input
              className="text-input monospace"
              value={opts.outputFile}
              onChange={e => set('outputFile', e.target.value)}
              placeholder="auto-generated from folder name"
            />
          </div>
        </div>

        {/* Options */}
        <div className="card">
          <p className="card-title">Options</p>

          <div className="field">
            <label className="field-label">Verification Hash</label>
            <select
              className="select"
              value={opts.hashAlgorithm}
              onChange={e => set('hashAlgorithm', e.target.value)}
              disabled={opts.foldersOnly}
            >
              {HASH_ALGORITHMS.map(h => (
                <option key={h.value} value={h.value}>{h.label}</option>
              ))}
            </select>
          </div>

          <div className="field">
            <label className="field-label">Exclude Extensions</label>
            <input
              className="text-input"
              value={opts.excludedExts}
              onChange={e => set('excludedExts', e.target.value)}
              placeholder="tmp, log, cache"
            />
          </div>

          <div style={{ marginTop: 8 }}>
            <Toggle label="Folders only (no files)" checked={opts.foldersOnly} onChange={v => {
              set('foldersOnly', v)
              if (v) { set('createXLSX', false); set('preserveZeros', false); set('deleteCSV', false) }
            }} />
            {opts.foldersOnly && (
              <div className="field" style={{ marginLeft: 20, marginTop: 4 }}>
                <label className="field-label">Max depth (0 = all levels)</label>
                <input
                  className="text-input"
                  type="number"
                  min={0}
                  max={99}
                  value={opts.folderDepth}
                  onChange={e => set('folderDepth', Math.max(0, parseInt(e.target.value) || 0))}
                  style={{ width: 80 }}
                />
              </div>
            )}
            <Toggle label="Exclude hidden files"        checked={opts.excludeHidden} onChange={v => set('excludeHidden', v)} />
            <Toggle label="Exclude common system files" checked={opts.excludeSystem} onChange={v => set('excludeSystem', v)} disabled={opts.foldersOnly} />
            <Toggle label="Create XLSX after scan"      checked={opts.createXLSX}   disabled={opts.foldersOnly} onChange={v => {
              set('createXLSX', v)
              if (!v) { set('preserveZeros', false); set('deleteCSV', false) }
              else     { set('preserveZeros', true);  set('deleteCSV', true)  }
            }} />
            <Toggle label="Preserve leading zeros in XLSX" checked={opts.preserveZeros && opts.createXLSX}
              onChange={v => set('preserveZeros', v)} disabled={!opts.createXLSX || opts.foldersOnly} indent />
            <Toggle label="Delete CSV after XLSX created"  checked={opts.deleteCSV && opts.createXLSX}
              onChange={v => set('deleteCSV', v)} disabled={!opts.createXLSX || opts.foldersOnly} indent />
          </div>
        </div>

        <div className="card">
          <p className="card-title">Agency Template</p>
          <Toggle label="Use agency content-list headers" checked={opts.agencyTemplate} onChange={v => {
            setOpts(o => ({
              ...o,
              agencyTemplate: v,
              foldersOnly: v ? false : o.foldersOnly,
              agencyFields: { ...DEFAULT_AGENCY_FIELDS, ...o.agencyFields },
            }))
          }} />
          {opts.agencyTemplate && (
            <>
              {agencyMissingRCSeries && (
                <p className="danger-text" style={{ margin: '4px 0 8px' }}>
                  RC Series is required for agency template output.
                </p>
              )}
              {agencyFieldError && (
                <p className="danger-text" style={{ margin: '4px 0 8px' }}>
                  {agencyFieldError}
                </p>
              )}
              <div className="field-grid">
                <div className="field">
                  <label className="field-label">RG</label>
                  <input className="text-input" value={opts.agencyFields.rg} onChange={e => setAgency('rg', e.target.value)} placeholder="4 digits, e.g. 1325" inputMode="numeric" maxLength={4} />
                </div>
                <div className="field">
                  <label className="field-label">SG</label>
                  <input className="text-input" value={opts.agencyFields.sg} onChange={e => setAgency('sg', e.target.value)} placeholder="3 digits, e.g. 1 -> 001" inputMode="numeric" maxLength={3} />
                </div>
                <div className="field">
                  <label className="field-label">Series</label>
                  <input className="text-input" value={opts.agencyFields.series} onChange={e => setAgency('series', e.target.value)} placeholder="3 digits, e.g. 35 -> 035" inputMode="numeric" maxLength={3} />
                </div>
                <div className="field">
                  <label className="field-label">RC Series</label>
                  <input className="text-input" value={opts.agencyFields.rcSeries} onChange={e => setAgency('rcSeries', e.target.value)} placeholder="required" />
                </div>
                <div className="field">
                  <label className="field-label">Department / Organization</label>
                  <input className="text-input" value={opts.agencyFields.deptOrganization} onChange={e => setAgency('deptOrganization', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">RC Series Name</label>
                  <input className="text-input" value={opts.agencyFields.rcSeriesName} onChange={e => setAgency('rcSeriesName', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">Division</label>
                  <input className="text-input" value={opts.agencyFields.division} onChange={e => setAgency('division', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">Section</label>
                  <input className="text-input" value={opts.agencyFields.section} onChange={e => setAgency('section', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">Unit</label>
                  <input className="text-input" value={opts.agencyFields.unit} onChange={e => setAgency('unit', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">Begin Date</label>
                  <input className="text-input" value={opts.agencyFields.beginDate} onChange={e => setAgency('beginDate', e.target.value)} placeholder="YYYY or MM/YYYY" />
                </div>
                <div className="field">
                  <label className="field-label">End Date</label>
                  <input className="text-input" value={opts.agencyFields.endDate} onChange={e => setAgency('endDate', e.target.value)} placeholder="YYYY or MM/YYYY" />
                </div>
                <div className="field">
                  <label className="field-label">Material Type</label>
                  <input className="text-input" value={opts.agencyFields.materialType} onChange={e => setAgency('materialType', e.target.value)} placeholder="Born Digital" />
                </div>
                <div className="field">
                  <label className="field-label">Confidential</label>
                  <select className="select" value={opts.agencyFields.confidential} onChange={e => setAgency('confidential', e.target.value)}>
                    <option value="">blank</option>
                    <option value="No">No</option>
                    <option value="Yes">Yes</option>
                  </select>
                </div>
                <div className="field">
                  <label className="field-label">Disposition Date</label>
                  <input className="text-input" value={opts.agencyFields.dispositionDate} onChange={e => setAgency('dispositionDate', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">TD Number</label>
                  <input className="text-input" value={opts.agencyFields.tdNum} onChange={e => setAgency('tdNum', e.target.value)} />
                </div>
                <div className="field">
                  <label className="field-label">Box Number</label>
                  <input className="text-input" value={opts.agencyFields.boxNum} onChange={e => setAgency('boxNum', e.target.value)} placeholder="###.###" />
                </div>
                <div className="field">
                  <label className="field-label">Location ID</label>
                  <input className="text-input" value={opts.agencyFields.locationId} onChange={e => setAgency('locationId', e.target.value)} placeholder="drive, share, or storage location" />
                </div>
                <div className="field">
                  <label className="field-label">Record Level</label>
                  <input className="text-input" value={opts.agencyFields.recordLevel} onChange={e => setAgency('recordLevel', e.target.value)} placeholder="Item" />
                </div>
              </div>
              <div className="field">
                <label className="field-label">Description</label>
                <textarea className="text-input" value={opts.agencyFields.description} onChange={e => setAgency('description', e.target.value)} rows={2} />
              </div>
              <div className="field">
                <label className="field-label">Location Override</label>
                <input className="text-input" value={opts.agencyFields.location} onChange={e => setAgency('location', e.target.value)} placeholder="leave blank to use parent folder path" />
              </div>
              <div className="field">
                <label className="field-label">Comments</label>
                <textarea className="text-input" value={opts.agencyFields.comments} onChange={e => setAgency('comments', e.target.value)} rows={2} placeholder="optional constant note" />
              </div>
            </>
          )}
        </div>

        <div style={{ marginTop: 16 }}>
          <button className="btn btn-primary btn-lg" onClick={start} disabled={!canStart}>
            Start Scan
          </button>
        </div>
      </div>
    )
  }

  // ── Scanning ──────────────────────────────────────────────
  if (phase === 'scanning') {
    const pct = progress?.percent ?? 0
    const counting = progress?.phase !== 'Scanning'
    return createPortal(
      <div className="scan-overlay">
        <div className="scan-overlay-panel">
          <div className="screen-header" style={{ marginBottom: 16 }}>
            <h2 className="screen-title">{counting ? 'Counting…' : 'Scanning…'}</h2>
          </div>

          <div className="phase-badge">
            <span className="phase-dot" />
            {progress?.phase ?? 'Starting…'}
          </div>

          <ProgressBar percent={pct} animated={counting} />

          <div className="stat-grid" style={{ marginBottom: 12 }}>
            <div className="stat-block">
              <div className="stat-block-label">Progress</div>
              <div className="stat-block-value">
                {counting
                  ? `${fmtNum(progress?.files ?? 0)} files, ${fmtNum(progress?.directories ?? 0)} dirs`
                  : `${(pct * 100).toFixed(1)}%`}
              </div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">ETA</div>
              <div className="stat-block-value">{fmtETA(progress?.etaSecs ?? 0, progress?.phase ?? '')}</div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Files</div>
              <div className="stat-block-value">
                {fmtNum(progress?.files ?? 0)}
                {(progress?.totalFiles ?? 0) > 0 && ` / ${fmtNum(progress!.totalFiles)}`}
              </div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Bytes</div>
              <div className="stat-block-value">
                {fmtBytes(progress?.bytes ?? 0)}
                {(progress?.totalBytes ?? 0) > 0 && ` / ${fmtBytes(progress!.totalBytes)}`}
              </div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Directories</div>
              <div className="stat-block-value">{fmtNum(progress?.directories ?? 0)}</div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Filtered</div>
              <div className="stat-block-value">{fmtNum(progress?.filtered ?? 0)}</div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Elapsed</div>
              <div className="stat-block-value">{fmtSecs(progress?.elapsedSecs ?? 0)}</div>
            </div>
          </div>

          {progress?.currentItem && (
            <div className="current-file">{progress.currentItem}</div>
          )}

          <div style={{ marginTop: 16 }}>
            <button className="btn btn-danger" onClick={cancel}>Stop Scan</button>
          </div>
        </div>
      </div>,
      document.body
    )
  }

  // ── Error ─────────────────────────────────────────────────
  if (phase === 'error') {
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Scan Failed</h2>
        </div>
        <div className="card">
          <p className="danger-text" style={{ marginBottom: 16 }}>Error: {err}</p>
          <button className="btn btn-outline" onClick={() => reset()}>Back to Settings</button>
        </div>
      </div>
    )
  }

  // ── Done ──────────────────────────────────────────────────
  if (!result) return null

  return (
    <div>
      <div className="screen-header">
        <h2 className="screen-title">Scan Complete</h2>
        <p className="screen-subtitle success-text">
          {fmtNum(result.files)} files · {fmtBytes(result.bytes)} · {fmtSecs(result.elapsedSecs)}
        </p>
      </div>

      {/* Summary stats */}
      <div className="card">
        <p className="card-title">Summary</p>
        <div className="stat-grid" style={{ marginBottom: 16 }}>
          <div className="stat-block">
            <div className="stat-block-label">Files</div>
            <div className="stat-block-value success-text">{fmtNum(result.files)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Directories</div>
            <div className="stat-block-value">{fmtNum(result.directories)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Total Size</div>
            <div className="stat-block-value">{fmtBytes(result.bytes)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Filtered Out</div>
            <div className="stat-block-value">{fmtNum(result.filtered)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Elapsed</div>
            <div className="stat-block-value">{fmtSecs(result.elapsedSecs)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Hash Workers</div>
            <div className="stat-block-value">{result.hashWorkers}</div>
          </div>
        </div>

        <div className="stat-row"><span className="stat-row-label">Source</span><span className="stat-row-value">{result.sourceName}</span></div>
        <div className="stat-row"><span className="stat-row-label">Output</span><span className="stat-row-value">{result.outputPath}</span></div>
        <div className="stat-row"><span className="stat-row-label">CSV parts</span><span className="stat-row-value">{result.csvPartCount}</span></div>
        {result.xlsxPath && <div className="stat-row"><span className="stat-row-label">XLSX</span><span className="stat-row-value">{result.xlsxPath}</span></div>}
        {result.reportPath && <div className="stat-row"><span className="stat-row-label">Report</span><span className="stat-row-value">{result.reportPath}</span></div>}
        <div className="stat-row"><span className="stat-row-label">Hash</span><span className="stat-row-value">{result.hashAlgorithm.toUpperCase()}</span></div>
        <div className="stat-row"><span className="stat-row-label">Hidden filtered</span><span className="stat-row-value">{fmtNum(result.filteredHidden)}</span></div>
        <div className="stat-row"><span className="stat-row-label">System filtered</span><span className="stat-row-value">{fmtNum(result.filteredSystem)}</span></div>
        <div className="stat-row"><span className="stat-row-label">Ext filtered</span><span className="stat-row-value">{fmtNum(result.filteredExts)}</span></div>
        <div className="stat-row"><span className="stat-row-label">Errors skipped</span><span className="stat-row-value">{fmtNum(result.errors)}</span></div>

        <div className="result-actions">
          <button className="btn btn-primary" onClick={() => {
            const p = result.outputPath
            const dir = p.substring(0, Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\')))
            OpenPath(dir || p)
          }}>Open Output Folder</button>
          {result.xlsxPath && <button className="btn btn-outline" onClick={() => OpenPath(result.xlsxPath)}>Open XLSX</button>}
          {result.reportPath && <button className="btn btn-outline" onClick={() => OpenPath(result.reportPath)}>Open Report</button>}
          <button className="btn btn-ghost" onClick={() => reset(true)}>New Scan</button>
        </div>
      </div>

      {/* Top by count */}
      {result.topByCount?.length > 0 && (
        <div className="card">
          <p className="card-title">Top Extensions by Count</p>
          <table className="ext-table">
            <thead>
              <tr><th>Extension</th><th>Files</th><th>Size</th></tr>
            </thead>
            <tbody>
              {result.topByCount.map(e => (
                <tr key={e.label}>
                  <td className="ext-label">{e.label || '(none)'}</td>
                  <td>{fmtNum(e.count)}</td>
                  <td>{fmtBytes(e.bytes)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Top by size */}
      {result.topBySize?.length > 0 && (
        <div className="card">
          <p className="card-title">Top Extensions by Size</p>
          <table className="ext-table">
            <thead>
              <tr><th>Extension</th><th>Size</th><th>Files</th></tr>
            </thead>
            <tbody>
              {result.topBySize.map(e => (
                <tr key={e.label}>
                  <td className="ext-label">{e.label || '(none)'}</td>
                  <td>{fmtBytes(e.bytes)}</td>
                  <td>{fmtNum(e.count)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Filtered samples */}
      {result.filteredSamples?.length > 0 && (
        <div className="card">
          <p className="card-title">Filtered Samples</p>
          {result.filteredSamples.map((s, i) => (
            <div key={i} className="current-file" style={{ marginBottom: 4 }}>{s}</div>
          ))}
        </div>
      )}
    </div>
  )
}
