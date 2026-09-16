import { useEffect, useRef, useState } from 'react'
import { CancelCloneCompare, OpenPath, RevealPath, ResumeCloneWithDriveB, StartCloneCompare } from '../../wailsjs/go/main/App'
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import FolderPicker from '../components/FolderPicker'
import ProgressBar from '../components/ProgressBar'
import {
  CloneCompareOptions, CloneDonePayload,
  CloneProgressPayload, DiffRowPayload,
  HASH_ALGORITHMS,
} from '../types'

type Phase = 'idle' | 'running' | 'awaiting-drive-b' | 'done' | 'error' | 'canceled'

const DIFF_ROW_CAP = 5000
const SPEED_HISTORY_LEN = 240  // 60 seconds at 250 ms ticks

function fmtNum(n: number) { return n.toLocaleString() }

function fmtSecs(s: number) {
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${(s % 60).toFixed(0)}s`
}

function fmtBytes(b: number): string {
  if (b <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.min(Math.floor(Math.log2(b) / 10), units.length - 1)
  return `${(b / Math.pow(1024, i)).toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function fmtSpeed(bps: number): string {
  if (bps <= 0) return '— MB/s'
  if (bps >= 1e9) return `${(bps / 1e9).toFixed(1)} GB/s`
  if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} MB/s`
  return `${(bps / 1e3).toFixed(0)} KB/s`
}

function diffTypeBadge(type: string) {
  if (type.includes('missing'))   return 'diff-type-missing'
  if (type.includes('extra'))     return 'diff-type-extra'
  if (type.includes('moved'))     return 'diff-type-moved'
  if (type.includes('duplicate')) return 'diff-type-duplicate'
  if (type.includes('metadata'))  return 'diff-type-metadata'
  return 'diff-type-mismatch'
}

function diffPrimaryPath(row: DiffRowPayload) {
  if (row.diffType === 'moved/renamed') return `${row.pathA} → ${row.pathB}`
  return row.pathA || row.pathB
}

function phaseLabel(phase: string) {
  if (phase === 'scan-a') return 'Scanning 1st Drive'
  if (phase === 'scan-b') return 'Scanning 2nd Drive'
  if (phase === 'diff')   return 'Comparing Drives'
  return 'Starting…'
}

function phaseStep(phase: string) {
  if (phase === 'scan-a') return 0
  if (phase === 'scan-b') return 1
  if (phase === 'diff')   return 2
  return -1
}

function Sparkline({
  values,
  currentSpeed = 0,
  splitAt,
}: {
  values: number[]
  currentSpeed?: number
  splitAt?: number
}) {
  const LPAD = 50, BPAD = 18, WIDTH = 500, HEIGHT = 104
  const plotW = WIDTH - LPAD, plotH = HEIGHT - BPAD

  if (values.length < 2) return <svg width="100%" viewBox={`0 0 ${WIDTH} ${HEIGHT}`} />

  const alpha = 0.25
  const smoothed = values.reduce((acc, v) => {
    acc.push(acc.length === 0 ? v : alpha * v + (1 - alpha) * acc[acc.length - 1])
    return acc
  }, [] as number[])

  const sorted = [...smoothed].sort((a, b) => a - b)
  const p95 = sorted[Math.max(0, Math.ceil(0.95 * sorted.length) - 1)]
  const scaleMax = Math.max(p95, 1e5)
  const logMax = Math.log(scaleMax + 1)

  const toX = (i: number) => LPAD + (i / (smoothed.length - 1)) * plotW
  const toY = (v: number) => plotH - (Math.log(Math.min(v, scaleMax) + 1) / logMax) * plotH

  const linePath = smoothed.reduce((d, v, i) => {
    const x = toX(i), y = toY(v)
    if (i === 0) return `M ${x.toFixed(1)} ${y.toFixed(1)}`
    const px = toX(i - 1), py = toY(smoothed[i - 1])
    const cx = ((px + x) / 2).toFixed(1)
    return `${d} C ${cx} ${py.toFixed(1)} ${cx} ${y.toFixed(1)} ${x.toFixed(1)} ${y.toFixed(1)}`
  }, '')
  const fillPath = `${linePath} L ${toX(smoothed.length - 1)} ${plotH} L ${LPAD} ${plotH} Z`

  // Y axis ticks — logarithmically spaced candidates, pick those that fit
  const yTickCandidates = [1e3, 1e4, 5e4, 1e5, 5e5, 1e6, 5e6, 1e7, 5e7, 1e8, 2e8, 5e8, 1e9, 2e9, 5e9]
  const yTicks = yTickCandidates.filter(v => v <= scaleMax * 1.01 && toY(v) >= 2 && toY(v) <= plotH - 2)
  // Thin to ~4 ticks max
  const yStep = Math.max(1, Math.ceil(yTicks.length / 4))
  const displayYTicks = yTicks.filter((_, i) => i % yStep === 0)

  // X axis ticks — time labels
  const totalSecs = values.length * 0.25
  const xInterval = totalSecs <= 15 ? 5 : totalSecs <= 60 ? 15 : 30
  const xTicks: { i: number; label: string }[] = []
  for (let s = 0; s <= totalSecs; s += xInterval) {
    const idx = Math.round(s / 0.25)
    if (idx < values.length) xTicks.push({ i: idx, label: `${s}s` })
  }

  const splitX = splitAt !== undefined && splitAt > 0 && splitAt < smoothed.length ? toX(splitAt) : null
  const currentY = currentSpeed > 0 ? toY(currentSpeed) : null

  return (
    <svg width="100%" viewBox={`0 0 ${WIDTH} ${HEIGHT}`} style={{ display: 'block' }}>
      <defs>
        <linearGradient id="spk-fill" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.35" />
          <stop offset="100%" stopColor="var(--accent)" stopOpacity="0.02" />
        </linearGradient>
        <clipPath id="spk-clip">
          <rect x={LPAD} y={0} width={plotW} height={plotH} />
        </clipPath>
      </defs>

      {/* Y grid lines + labels */}
      {displayYTicks.map(v => {
        const y = toY(v)
        return (
          <g key={v}>
            <line x1={LPAD} y1={y} x2={WIDTH} y2={y} stroke="var(--border)" strokeWidth={0.5} />
            <text x={LPAD - 5} y={y + 3.5} fontSize={9} fill="var(--text-muted)"
              textAnchor="end" style={{ userSelect: 'none' }}>
              {fmtSpeed(v)}
            </text>
          </g>
        )
      })}

      {/* Plot area */}
      <g clipPath="url(#spk-clip)">
        <path d={fillPath} fill="url(#spk-fill)" />
        <path d={linePath} fill="none" stroke="var(--accent)" strokeWidth={2} strokeLinecap="round" />

        {/* Phase split */}
        {splitX !== null && (
          <>
            <line x1={splitX} y1={0} x2={splitX} y2={plotH}
              stroke="var(--text-muted)" strokeWidth={1} strokeDasharray="4,3" opacity={0.5} />
            <text x={splitX - 4} y={11} fontSize={8} fill="var(--text-muted)"
              textAnchor="end" style={{ userSelect: 'none' }}>A</text>
            <text x={splitX + 4} y={11} fontSize={8} fill="var(--text-muted)"
              textAnchor="start" style={{ userSelect: 'none' }}>B</text>
          </>
        )}

        {/* Current speed line */}
        {currentY !== null && currentY > 2 && currentY < plotH - 2 && (
          <>
            <line x1={LPAD} y1={currentY} x2={WIDTH} y2={currentY}
              stroke="var(--accent)" strokeWidth={1} strokeDasharray="3,2" opacity={0.75} />
            <text x={WIDTH - 2} y={currentY - 4} fontSize={9} fill="var(--accent)"
              textAnchor="end" fontWeight={700} style={{ userSelect: 'none' }}>
              {fmtSpeed(currentSpeed)}
            </text>
          </>
        )}
      </g>

      {/* Axes */}
      <line x1={LPAD} y1={0} x2={LPAD} y2={plotH} stroke="var(--border)" strokeWidth={1} />
      <line x1={LPAD} y1={plotH} x2={WIDTH} y2={plotH} stroke="var(--border)" strokeWidth={1} />

      {/* X ticks + labels */}
      {xTicks.map(({ i, label }) => (
        <g key={i}>
          <line x1={toX(i)} y1={plotH} x2={toX(i)} y2={plotH + 3} stroke="var(--border)" strokeWidth={1} />
          <text x={toX(i)} y={plotH + 13} fontSize={9} fill="var(--text-muted)"
            textAnchor="middle" style={{ userSelect: 'none' }}>{label}</text>
        </g>
      ))}
    </svg>
  )
}

export default function CloneCompare() {
  const [opts, setOpts] = useState<CloneCompareOptions>({
    driveA: '', driveB: '', outputDir: '', hashAlgorithm: 'blake3', softCompare: false,
  })
  const [singleDriveMode, setSingleDriveMode] = useState(false)
  const [driveBPath, setDriveBPath]   = useState('')
  const [phase, setPhase]             = useState<Phase>('idle')
  const [clonePhase, setClonePhase]   = useState('')
  const [progress, setProgress]       = useState<CloneProgressPayload | null>(null)
  const [speedHistory, setSpeedHistory] = useState<number[]>([])
  const [speedSplitAt, setSpeedSplitAt] = useState<number | undefined>(undefined)
  const [diffRows, setDiffRows]       = useState<DiffRowPayload[]>([])
  const [hiddenRows, setHiddenRows]   = useState(0)
  const [result, setResult]           = useState<CloneDonePayload | null>(null)
  const [err, setErr]                 = useState('')
  const tableEndRef = useRef<HTMLDivElement>(null)
  const prevPhaseRef = useRef('')
  const speedLenRef  = useRef(0)

  useEffect(() => {
    return () => {
      EventsOff('clone:progress')
      EventsOff('clone:diff-row')
      EventsOff('clone:done')
      EventsOff('clone:error')
      EventsOff('clone:canceled')
      EventsOff('clone:awaiting-drive-b')
    }
  }, [])

  useEffect(() => {
    if (diffRows.length > 0) {
      tableEndRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [diffRows.length])

  const set = (key: keyof CloneCompareOptions, value: string) =>
    setOpts(o => ({ ...o, [key]: value }))

  const start = async () => {
    const effectiveDriveB = singleDriveMode ? '' : opts.driveB
    if (!singleDriveMode && opts.driveA.trim() === opts.driveB.trim()) {
      setErr('1st Drive and 2nd Drive must be different folders.')
      setPhase('error')
      return
    }
    setPhase('running')
    setClonePhase('')
    setProgress(null)
    setSpeedHistory([])
    setSpeedSplitAt(undefined)
    setDiffRows([])
    setHiddenRows(0)
    setDriveBPath('')
    setErr('')
    prevPhaseRef.current = ''
    speedLenRef.current  = 0

    EventsOn('clone:progress', (d: CloneProgressPayload) => {
      setClonePhase(d.phase)
      setProgress(d)
      if (d.phase === 'scan-a' || d.phase === 'scan-b') {
        if (prevPhaseRef.current === 'scan-a' && d.phase === 'scan-b') {
          setSpeedSplitAt(speedLenRef.current)
        }
        prevPhaseRef.current = d.phase
        const bps = d.bytesPerSec
        if (bps > 0) {  // skip counting-phase zeros
          setSpeedHistory(prev => {
            const next = [...prev, bps]
            const trimmed = next.length > SPEED_HISTORY_LEN ? next.slice(-SPEED_HISTORY_LEN) : next
            speedLenRef.current = trimmed.length
            return trimmed
          })
        }
      }
    })
    EventsOn('clone:diff-row', (d: DiffRowPayload) => {
      setDiffRows(prev => {
        if (prev.length >= DIFF_ROW_CAP) {
          setHiddenRows(h => h + 1)
          return prev
        }
        return [...prev, d]
      })
    })
    EventsOn('clone:done',     (d: CloneDonePayload) => { setResult(d); setPhase('done') })
    EventsOn('clone:error',    (msg: string)         => { setErr(msg); setPhase('error') })
    EventsOn('clone:canceled', ()                    => setPhase('canceled'))
    EventsOn('clone:awaiting-drive-b', ()            => {
      setSpeedSplitAt(speedLenRef.current)  // mark where Drive B scan will begin
      setPhase('awaiting-drive-b')
    })

    try {
      await StartCloneCompare({ ...opts, driveB: effectiveDriveB })
    } catch (e: any) {
      EventsOff('clone:progress'); EventsOff('clone:diff-row')
      EventsOff('clone:done');     EventsOff('clone:error')
      EventsOff('clone:canceled'); EventsOff('clone:awaiting-drive-b')
      setErr(String(e))
      setPhase('error')
    }
  }

  const resumeWithDriveB = async () => {
    if (!driveBPath) return
    try {
      setPhase('running')
      await ResumeCloneWithDriveB(driveBPath)
    } catch (e: any) {
      setErr(String(e))
      setPhase('error')
    }
  }

  const cancel = () => CancelCloneCompare().catch(() => {})

  const reset = () => {
    EventsOff('clone:progress'); EventsOff('clone:diff-row')
    EventsOff('clone:done');     EventsOff('clone:error')
    EventsOff('clone:canceled'); EventsOff('clone:awaiting-drive-b')
    setPhase('idle'); setProgress(null); setSpeedHistory([]); setSpeedSplitAt(undefined)
    setDiffRows([]); setHiddenRows(0); setResult(null); setErr(''); setDriveBPath('')
    prevPhaseRef.current = ''; speedLenRef.current = 0
  }

  const drivesMismatch = !singleDriveMode && !!(opts.driveA && opts.driveB && opts.driveA === opts.driveB)
  const canStart = opts.driveA && opts.outputDir && !drivesMismatch &&
    (singleDriveMode || !!opts.driveB)

  // ── Form ──────────────────────────────────────────────────
  if (phase === 'idle' || phase === 'canceled') {
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Clone Compare</h2>
          <p className="screen-subtitle">Scan two drives and diff the results to verify a clone.</p>
        </div>

        {phase === 'canceled' && (
          <div className="card" style={{ marginBottom: 16 }}>
            <span className="danger-text">Comparison was stopped.</span>
          </div>
        )}

        <div className="card">
          <p className="card-title">Drives</p>

          <div className="field" style={{ marginBottom: 12 }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13 }}>
              <input
                type="checkbox"
                checked={singleDriveMode}
                onChange={e => setSingleDriveMode(e.target.checked)}
              />
              Single drive mode — eject &amp; swap between scans
            </label>
          </div>

          <FolderPicker label="1st Drive (Source)" value={opts.driveA} onChange={v => set('driveA', v)} />

          {!singleDriveMode && (
            <>
              <FolderPicker label="2nd Drive (Clone)" value={opts.driveB} onChange={v => set('driveB', v)} />
              {drivesMismatch && (
                <p className="danger-text" style={{ margin: '4px 0 8px' }}>
                  1st Drive and 2nd Drive must be different folders.
                </p>
              )}
            </>
          )}

          {singleDriveMode && (
            <p className="info-text" style={{ margin: '4px 0 8px', fontSize: 12 }}>
              Drive A will be scanned first. You will be prompted to eject it and insert Drive B before the second scan begins.
            </p>
          )}

          <FolderPicker label="Output Folder" value={opts.outputDir} onChange={v => set('outputDir', v)} />
          <div className="field">
            <label className="field-label">Hash Algorithm</label>
            <select className="select" value={opts.hashAlgorithm} onChange={e => set('hashAlgorithm', e.target.value)}>
              {HASH_ALGORITHMS.filter(h => h.value !== 'off').map(h => (
                <option key={h.value} value={h.value}>{h.label}</option>
              ))}
            </select>
          </div>
          <div className="field" style={{ marginTop: 12 }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', fontSize: 13 }}>
              <input
                type="checkbox"
                checked={opts.softCompare}
                onChange={e => setOpts(o => ({ ...o, softCompare: e.target.checked }))}
              />
              Soft compare — match PDFs by content, ignoring embedded document IDs
            </label>
            {opts.softCompare && (
              <p className="info-text" style={{ margin: '4px 0 0 24px', fontSize: 12 }}>
                For PDFs that were independently exported from the same source, document IDs differ
                even when the visible content is identical. Soft compare detects these as
                "Metadata Clone" instead of "Not a Clone".
              </p>
            )}
          </div>
        </div>

        <div className="info-text" style={{ margin: '12px 0' }}>
          Scans both drives sequentially using the same hash algorithm, then diffs the CSV outputs.
          Files are compared by relative path, size, and hash value.
        </div>

        <button className="btn btn-primary btn-lg" onClick={start} disabled={!canStart}>
          Start Compare
        </button>
      </div>
    )
  }

  // ── Awaiting Drive B ──────────────────────────────────────
  if (phase === 'awaiting-drive-b') {
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Swap Drives</h2>
          <p className="screen-subtitle">Drive A scan complete. Ready for Drive B.</p>
        </div>
        <div className="card">
          <p className="card-title">Action Required</p>
          <ol style={{ margin: '0 0 16px', paddingLeft: 20, lineHeight: 1.8, fontSize: 13 }}>
            <li>Eject Drive A from your computer.</li>
            <li>Connect Drive B (the clone drive).</li>
            <li>Browse to Drive B below, then click Continue.</li>
          </ol>
          <FolderPicker label="2nd Drive (Clone)" value={driveBPath} onChange={setDriveBPath} />
          <div style={{ display: 'flex', gap: 8, marginTop: 16 }}>
            <button className="btn btn-primary" onClick={resumeWithDriveB} disabled={!driveBPath}>
              Continue
            </button>
            <button className="btn btn-danger" onClick={cancel}>Cancel</button>
          </div>
        </div>
      </div>
    )
  }

  // ── Running ───────────────────────────────────────────────
  if (phase === 'running') {
    const step     = phaseStep(clonePhase)
    const pct      = progress?.percent ?? 0
    const isScan   = clonePhase === 'scan-a' || clonePhase === 'scan-b'
    const counting = isScan && (progress?.subPhase ?? '') === 'Counting'
    const scanning = isScan && (progress?.subPhase ?? '') === 'Scanning'

    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Comparing…</h2>
          <p className="screen-subtitle">{phaseLabel(clonePhase)}</p>
        </div>

        {/* Phase step indicator */}
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          {(['scan-a', 'scan-b', 'diff'] as const).map((s, i) => {
            const done    = step > i
            const active  = step === i
            return (
              <div key={s} style={{
                flex: 1,
                padding: '6px 10px',
                borderRadius: 'var(--radius-sm)',
                fontSize: 12,
                fontWeight: 600,
                textAlign: 'center',
                background: done ? 'var(--success-bg)' : active ? 'var(--accent-light)' : 'var(--warning-bg)',
                color:      done ? 'var(--success)'    : active ? 'var(--accent)'       : 'var(--warning)',
                border: `1px solid ${done ? 'var(--success)' : active ? 'var(--accent)' : 'var(--warning)'}`,
              }}>
                {done ? '✓' : active ? '▶' : '○'} {phaseLabel(s)}
              </div>
            )
          })}
        </div>

        {/* Scan-phase progress */}
        {isScan && (
          <div className="card">
            {counting ? (
              <>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6, fontSize: 12, color: 'var(--text-muted)' }}>
                  <span>Counting files…</span>
                  {(progress?.files ?? 0) > 0 && <span>{fmtNum(progress!.files)} found</span>}
                </div>
                <ProgressBar percent={0} animated />
              </>
            ) : (
              <>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                  <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                    {scanning ? 'Scanning' : 'Starting…'}
                  </span>
                  <span style={{ fontSize: 20, fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>
                    {(pct * 100).toFixed(1)}%
                  </span>
                </div>
                <ProgressBar percent={pct} animated={false} />
              </>
            )}

            {/* Stat grid */}
            <div className="stat-grid" style={{ marginTop: 14, marginBottom: 12 }}>
              <div className="stat-block">
                <div className="stat-block-label">Files</div>
                <div className="stat-block-value" style={{ fontSize: 13 }}>
                  {fmtNum(progress?.files ?? 0)}
                  {(progress?.totalFiles ?? 0) > 0 && (
                    <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>
                      {' '}/ {fmtNum(progress!.totalFiles)}
                    </span>
                  )}
                </div>
              </div>
              <div className="stat-block">
                <div className="stat-block-label">Data</div>
                <div className="stat-block-value" style={{ fontSize: 13 }}>
                  {fmtBytes(progress?.bytes ?? 0)}
                  {(progress?.totalBytes ?? 0) > 0 && (
                    <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>
                      {' '}/ {fmtBytes(progress!.totalBytes)}
                    </span>
                  )}
                </div>
              </div>
              <div className="stat-block">
                <div className="stat-block-label">Speed</div>
                <div className="stat-block-value" style={{ fontSize: 13 }}>
                  {fmtSpeed(speedHistory.length > 0
                    ? speedHistory.slice(-8).reduce((a, b) => a + b, 0) / Math.min(8, speedHistory.length)
                    : 0)}
                </div>
              </div>
              <div className="stat-block">
                <div className="stat-block-label">ETA</div>
                <div className="stat-block-value" style={{ fontSize: 13 }}>
                  {(progress?.etaSecs ?? 0) > 0 ? fmtSecs(progress!.etaSecs) : '—'}
                </div>
              </div>
              <div className="stat-block">
                <div className="stat-block-label">Elapsed</div>
                <div className="stat-block-value" style={{ fontSize: 13 }}>
                  {(progress?.elapsedSecs ?? 0) > 0 ? fmtSecs(progress!.elapsedSecs) : '—'}
                </div>
              </div>
            </div>

            {/* Speed sparkline */}
            {speedHistory.length > 1 && (
              <div style={{ marginBottom: 10 }}>
                <div style={{ fontSize: 11, color: 'var(--text-muted)', marginBottom: 8 }}>
                  Read speed — {(speedHistory.length * 0.25).toFixed(0)}s recorded
                </div>
                <div style={{ margin: '0 -4px' }}>
                  <Sparkline
                    values={speedHistory}
                    splitAt={speedSplitAt}
                    currentSpeed={speedHistory.slice(-8).reduce((a, b) => a + b, 0) / Math.min(8, speedHistory.length)}
                  />
                </div>
              </div>
            )}

            {/* Current file */}
            {progress?.currentItem && (
              <div className="current-file" style={{ marginBottom: 10 }}>
                {progress.currentItem}
              </div>
            )}

            <button className="btn btn-danger" onClick={cancel}>Stop</button>
          </div>
        )}

        {/* Diff-phase progress */}
        {clonePhase === 'diff' && (
          <div className="card">
            <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
              <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Comparing</span>
              <span style={{ fontSize: 20, fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>
                {(pct * 100).toFixed(1)}%
              </span>
            </div>
            <ProgressBar percent={pct} animated={pct === 0} />

            <div className="stat-grid" style={{ marginTop: 14, marginBottom: 12 }}>
              <div className="stat-block">
                <div className="stat-block-label">Compared</div>
                <div className="stat-block-value">{fmtNum(progress?.compared ?? 0)}</div>
              </div>
              <div className="stat-block">
                <div className="stat-block-label">Total</div>
                <div className="stat-block-value">{fmtNum(progress?.total ?? 0)}</div>
              </div>
              <div className="stat-block">
                <div className="stat-block-label">Differences</div>
                <div className="stat-block-value" style={{ color: (progress?.differences ?? 0) > 0 ? 'var(--danger)' : 'inherit' }}>
                  {fmtNum(progress?.differences ?? 0)}
                </div>
              </div>
            </div>

            {progress?.currentItem && (
              <div className="current-file" style={{ marginBottom: 10 }}>{progress.currentItem}</div>
            )}

            <button className="btn btn-danger" onClick={cancel}>Stop</button>
          </div>
        )}

        {/* Starting (no phase yet) */}
        {!isScan && clonePhase !== 'diff' && (
          <div className="card">
            <ProgressBar percent={0} animated />
            <button className="btn btn-danger" style={{ marginTop: 12 }} onClick={cancel}>Stop</button>
          </div>
        )}

        {/* Live diff table */}
        {diffRows.length > 0 && (
          <div className="card">
            <p className="card-title">
              Live Differences ({fmtNum(diffRows.length)}{hiddenRows > 0 ? ` + ${fmtNum(hiddenRows)} more` : ''})
            </p>
            <div className="diff-table-wrap">
              <table className="diff-table">
                <thead>
                  <tr>
                    <th>Type</th><th>Path</th><th>Size A</th><th>Size B</th>
                  </tr>
                </thead>
                <tbody>
                  {diffRows.map((row, i) => (
                    <tr key={i}>
                      <td><span className={`diff-type-badge ${diffTypeBadge(row.diffType)}`}>{row.diffType}</span></td>
                      <td title={diffPrimaryPath(row)}>{diffPrimaryPath(row)}</td>
                      <td>{row.sizeA}</td>
                      <td>{row.sizeB}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <div ref={tableEndRef} />
            </div>
          </div>
        )}
      </div>
    )
  }

  // ── Error ─────────────────────────────────────────────────
  if (phase === 'error') {
    return (
      <div>
        <div className="screen-header"><h2 className="screen-title">Compare Failed</h2></div>
        <div className="card">
          <p className="danger-text" style={{ marginBottom: 16 }}>Error: {err}</p>
          <button className="btn btn-outline" onClick={reset}>Back to Settings</button>
        </div>
      </div>
    )
  }

  // ── Done ──────────────────────────────────────────────────
  if (!result) return null
  const isExact    = result.verdict === 'Exact Clone'
  const isContent  = result.verdict === 'Content Clone'
  const isMetadata = result.verdict === 'Metadata Clone'
  const isNotClone = result.verdict === 'Not a Clone'
  const verdictColor = isExact ? 'var(--success)' : isNotClone ? 'var(--danger)' : 'var(--warning)'
  const verdictIcon  = isExact ? '✓' : isNotClone ? '✗' : '≈'

  return (
    <div>
      <div className="screen-header">
        <h2 className="screen-title">Compare Complete</h2>
        <p className="screen-subtitle" style={{ color: verdictColor, fontWeight: 700 }}>
          {verdictIcon} {result.verdict}
        </p>
      </div>

      <div className="card">
        <p className="card-title">Results</p>
        <div className="stat-grid" style={{ marginBottom: 16 }}>
          <div className="stat-block">
            <div className="stat-block-label">Exact Matches</div>
            <div className="stat-block-value">{fmtNum(result.compared)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Moved / Renamed</div>
            <div className="stat-block-value">{fmtNum(result.movedFiles)}</div>
          </div>
          {result.softCompare && (
            <div className="stat-block">
              <div className="stat-block-label">Metadata-only (PDF IDs)</div>
              <div className="stat-block-value" style={{ color: isMetadata ? 'var(--warning)' : 'inherit' }}>
                {fmtNum(result.metadataOnlyDiffs)}
              </div>
            </div>
          )}
          <div className="stat-block">
            <div className="stat-block-label">Hash Mismatches</div>
            <div className="stat-block-value" style={{ color: result.hashMismatches > 0 ? 'var(--danger)' : 'inherit' }}>
              {fmtNum(result.hashMismatches)}
            </div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Missing (no match)</div>
            <div className="stat-block-value" style={{ color: result.missingNoMatch > 0 ? 'var(--danger)' : 'inherit' }}>
              {fmtNum(result.missingNoMatch)}
            </div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Extra (no match)</div>
            <div className="stat-block-value" style={{ color: result.extraNoMatch > 0 ? 'var(--danger)' : 'inherit' }}>
              {fmtNum(result.extraNoMatch)}
            </div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Duplicates on Clone</div>
            <div className="stat-block-value">{fmtNum(result.duplicatesOnB)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">System Excluded</div>
            <div className="stat-block-value">{fmtNum(result.excludedSystem)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Hash Algorithm</div>
            <div className="stat-block-value">{result.hashAlgorithm.toUpperCase()}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Elapsed</div>
            <div className="stat-block-value">{fmtSecs(result.elapsedSecs)}</div>
          </div>
        </div>

        {result.diffPath   && <div className="stat-row"><span className="stat-row-label">Diff CSV</span><span className="stat-row-value">{result.diffPath}</span></div>}
        {result.reportPath && <div className="stat-row"><span className="stat-row-label">Report</span><span className="stat-row-value">{result.reportPath}</span></div>}

        <div className="result-actions">
          {result.diffPath   && <button className="btn btn-primary" onClick={() => OpenPath(result.diffPath)}>Open Diff CSV</button>}
          {result.reportPath && <button className="btn btn-outline" onClick={() => OpenPath(result.reportPath)}>Open Report</button>}
          <button className="btn btn-ghost" onClick={reset}>New Compare</button>
        </div>
      </div>

      {diffRows.length > 0 && (() => {
        const alarmingRows = diffRows.filter(r => r.diffType.includes('no match'))
        const otherRows    = diffRows.filter(r => !r.diffType.includes('no match'))
        const diffTable = (rows: DiffRowPayload[], alarming: boolean) => (
          <div className="diff-table-wrap">
            <table className="diff-table">
              <thead>
                <tr>
                  <th>Type</th><th>Path</th><th>Size A</th><th>Size B</th><th>Hash A</th><th>Hash B</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row, i) => (
                  <tr key={i} className={alarming ? 'diff-row-alarming' : ''}>
                    <td><span className={`diff-type-badge ${diffTypeBadge(row.diffType)}`}>{row.diffType}</span></td>
                    <td title={diffPrimaryPath(row)}>{diffPrimaryPath(row)}</td>
                    <td>{row.sizeA}</td>
                    <td>{row.sizeB}</td>
                    <td title={row.hashA}>{row.hashA ? row.hashA.slice(0, 12) + '…' : ''}</td>
                    <td title={row.hashB}>{row.hashB ? row.hashB.slice(0, 12) + '…' : ''}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
        return (
          <>
            {alarmingRows.length > 0 && (
              <div className="diff-section-alarming">
                <div className="diff-section-alarming-header">
                  ⚠ Alarming — No Hash Match ({fmtNum(alarmingRows.length)})
                </div>
                {diffTable(alarmingRows, true)}
              </div>
            )}
            {otherRows.length > 0 && (
              <div className="card">
                <p className="card-title">
                  Other Differences ({fmtNum(otherRows.length)}
                  {hiddenRows > 0 && ` shown · ${fmtNum(hiddenRows)} more in CSV`})
                </p>
                {diffTable(otherRows, false)}
              </div>
            )}
          </>
        )
      })()}
    </div>
  )
}
