import { useEffect, useState } from 'react'
import { CancelEmailCopy, EmailExtensions, OpenPath, RevealPath, StartEmailCopy } from '../../wailsjs/go/main/App'
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime'
import FolderPicker from '../components/FolderPicker'
import ProgressBar from '../components/ProgressBar'
import { EmailDonePayload, EmailProgressPayload } from '../types'

type Phase = 'idle' | 'copying' | 'done' | 'error' | 'canceled'

function fmtNum(n: number) { return n.toLocaleString() }
function fmtSecs(s: number) {
  if (s < 60) return `${s.toFixed(1)}s`
  const m = Math.floor(s / 60)
  return `${m}m ${(s % 60).toFixed(0)}s`
}

export default function EmailCopy() {
  const [extensions, setExtensions] = useState<string[]>([])
  const [source, setSource] = useState('')
  const [dest, setDest]     = useState('')
  const [phase, setPhase]   = useState<Phase>('idle')
  const [progress, setProgress] = useState<EmailProgressPayload | null>(null)
  const [result, setResult] = useState<EmailDonePayload | null>(null)
  const [err, setErr]       = useState('')

  useEffect(() => {
    return () => {
      EventsOff('email:progress')
      EventsOff('email:done')
      EventsOff('email:error')
      EventsOff('email:canceled')
    }
  }, [])

  useEffect(() => {
    EmailExtensions().then(setExtensions).catch(() => setExtensions([]))
  }, [])

  const start = async () => {
    if (!source || !dest) return
    setPhase('copying')
    setProgress(null)
    setErr('')

    EventsOn('email:progress', (d: EmailProgressPayload) => setProgress(d))
    EventsOn('email:done',     (d: EmailDonePayload)     => { setResult(d); setPhase('done') })
    EventsOn('email:error',    (msg: string)             => { setErr(msg); setPhase('error') })
    EventsOn('email:canceled', ()                        => setPhase('canceled'))

    try {
      await StartEmailCopy(source, dest)
    } catch (e: any) {
      EventsOff('email:progress'); EventsOff('email:done')
      EventsOff('email:error');    EventsOff('email:canceled')
      setErr(String(e))
      setPhase('error')
    }
  }

  const cancel = () => CancelEmailCopy().catch(() => {})

  const reset = () => {
    EventsOff('email:progress'); EventsOff('email:done')
    EventsOff('email:error');    EventsOff('email:canceled')
    setPhase('idle'); setProgress(null); setResult(null); setErr('')
  }

  // ── Form ──────────────────────────────────────────────────
  if (phase === 'idle' || phase === 'canceled') {
    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Email Copy</h2>
          <p className="screen-subtitle">Copy email files to a destination, preserving folder structure.</p>
        </div>

        {phase === 'canceled' && (
          <div className="card" style={{ marginBottom: 16 }}>
            <span className="danger-text">Copy was stopped.</span>
          </div>
        )}

        <div className="card">
          <p className="card-title">Folders</p>
          <FolderPicker label="Source Folder"      value={source} onChange={setSource} />
          <FolderPicker label="Destination Folder" value={dest}   onChange={setDest} />

          <div className="info-text" style={{ marginTop: 12, marginBottom: 16 }}>
            Copies {extensions.length > 0 ? extensions.join(', ') : 'recognized email'} files and writes a manifest CSV.
          </div>

          <button className="btn btn-primary btn-lg" onClick={start} disabled={!source || !dest}>
            Start Copy
          </button>
        </div>
      </div>
    )
  }

  // ── Copying ───────────────────────────────────────────────
  if (phase === 'copying') {
    const isCopying = progress?.phase === 'copying'
    const pct = isCopying && (progress?.total ?? 0) > 0
      ? progress!.copied / progress!.total
      : 0

    return (
      <div>
        <div className="screen-header">
          <h2 className="screen-title">Copying…</h2>
        </div>
        <div className="card">
          <div className="phase-badge">
            <span className="phase-dot" />
            {isCopying ? 'Copying email files' : 'Scanning for email files'}…
          </div>

          <ProgressBar percent={pct} animated={!isCopying || pct === 0} />

          <div className="stat-grid" style={{ marginBottom: 12 }}>
            <div className="stat-block">
              <div className="stat-block-label">Copied</div>
              <div className="stat-block-value">{fmtNum(progress?.copied ?? 0)}</div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Total</div>
              <div className="stat-block-value">{fmtNum(progress?.total ?? 0)}</div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Scanned</div>
              <div className="stat-block-value">{fmtNum(progress?.scanned ?? 0)}</div>
            </div>
            <div className="stat-block">
              <div className="stat-block-label">Matched</div>
              <div className="stat-block-value">{fmtNum(progress?.matched ?? 0)}</div>
            </div>
          </div>

          <div className="stat-row"><span className="stat-row-label">Source</span><span className="stat-row-value">{source}</span></div>
          <div className="stat-row"><span className="stat-row-label">Destination</span><span className="stat-row-value">{dest}</span></div>

          <button className="btn btn-danger" style={{ marginTop: 12 }} onClick={cancel}>Stop</button>
        </div>
      </div>
    )
  }

  // ── Error ─────────────────────────────────────────────────
  if (phase === 'error') {
    return (
      <div>
        <div className="screen-header"><h2 className="screen-title">Copy Failed</h2></div>
        <div className="card">
          <p className="danger-text" style={{ marginBottom: 16 }}>Error: {err}</p>
          <button className="btn btn-outline" onClick={reset}>Try Again</button>
        </div>
      </div>
    )
  }

  // ── Done ──────────────────────────────────────────────────
  if (!result) return null

  return (
    <div>
      <div className="screen-header">
        <h2 className="screen-title">Copy Complete</h2>
        <p className="screen-subtitle success-text">{fmtNum(result.copied)} files copied</p>
      </div>
      <div className="card">
        <p className="card-title">Results</p>
        <div className="stat-grid" style={{ marginBottom: 16 }}>
          <div className="stat-block">
            <div className="stat-block-label">Files Copied</div>
            <div className="stat-block-value success-text">{fmtNum(result.copied)}</div>
          </div>
          <div className="stat-block">
            <div className="stat-block-label">Elapsed</div>
            <div className="stat-block-value">{fmtSecs(result.elapsedSecs)}</div>
          </div>
        </div>

        <div className="stat-row"><span className="stat-row-label">Source</span><span className="stat-row-value">{result.sourceDir}</span></div>
        <div className="stat-row"><span className="stat-row-label">Destination</span><span className="stat-row-value">{result.destDir}</span></div>
        <div className="stat-row"><span className="stat-row-label">Manifest</span><span className="stat-row-value">{result.manifestPath}</span></div>

        <div className="result-actions">
          <button className="btn btn-primary" onClick={() => OpenPath(result.destDir)}>Open Destination</button>
          {result.manifestPath && (
            <button className="btn btn-outline" onClick={() => OpenPath(result.manifestPath)}>Open Manifest</button>
          )}
          <button className="btn btn-ghost" onClick={reset}>New Copy</button>
        </div>
      </div>
    </div>
  )
}
