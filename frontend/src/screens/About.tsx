import { useEffect, useState } from 'react'
import { CheckForUpdates, GetAppVersion, GetScanDefaults, RestartToApplyUpdate, SaveReleaseFolder } from '../../wailsjs/go/main/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import FolderPicker from '../components/FolderPicker'
import { UpdateStatus } from '../types'

export default function About() {
  const [version, setVersion] = useState('…')
  const [releaseFolder, setReleaseFolder] = useState('')
  const [updateStatus, setUpdateStatus] = useState<UpdateStatus | null>(null)
  const [updateError, setUpdateError] = useState('')

  useEffect(() => {
    GetAppVersion().then(setVersion).catch(() => setVersion('unknown'))
    GetScanDefaults().then(defaults => setReleaseFolder(defaults.releaseFolder || '')).catch(() => {})
  }, [])

  const saveAndCheckUpdates = async () => {
    setUpdateError('')
    setUpdateStatus(null)
    try {
      await SaveReleaseFolder(releaseFolder)
      const status = await CheckForUpdates()
      setUpdateStatus(status)
    } catch (err: any) {
      setUpdateStatus(null)
      setUpdateError(String(err))
    }
  }

  return (
    <div>
      <div className="screen-header">
        <h2 className="screen-title">About</h2>
        <p className="screen-subtitle">Content List Toolkit v{version}</p>
      </div>

      <div className="card">
        <p className="card-title">What it does</p>
        <p className="info-text" style={{ marginBottom: 12 }}>
          Fast recursive folder scanner for very large collections. Generate a full CSV inventory
          with optional XLSX export, copy email files to a destination while preserving folder
          structure, or compare two drives to verify a clone.
        </p>
        <p className="info-text">
          CSV output streams directly to disk — no in-memory table — so scans of millions of files
          are handled without running out of memory. Files are split automatically at 300&thinsp;000
          rows per part.
        </p>

        <div className="divider" />

        <div className="stat-row">
          <span className="stat-row-label">Version</span>
          <span className="stat-row-value">{version}</span>
        </div>
        <div className="stat-row">
          <span className="stat-row-label">Runtime</span>
          <span className="stat-row-value">Go + Wails v2 + React</span>
        </div>
        <div className="stat-row">
          <span className="stat-row-label">Hash algorithms</span>
          <span className="stat-row-value">BLAKE3, SHA-1, SHA-256</span>
        </div>
        <div className="stat-row">
          <span className="stat-row-label">CSV row limit</span>
          <span className="stat-row-value">300,000 rows / part (auto-split)</span>
        </div>
        <div className="stat-row">
          <span className="stat-row-label">GitHub</span>
          <span className="stat-row-value">
            <button
              className="btn btn-ghost"
              style={{ padding: 0, color: 'var(--accent)', fontWeight: 500, fontSize: 13 }}
              onClick={() => BrowserOpenURL('https://github.com/snyderb-de/content-list-toolkit')}
            >
              github.com/snyderb-de/content-list-toolkit
            </button>
          </span>
        </div>
      </div>

      <div className="card">
        <p className="card-title">Updates</p>
        <p className="info-text" style={{ marginBottom: 14 }}>
          Set the mapped network folder where the signed Windows executable is published for staff. The default checks X:\Apps\content-list-generator.exe.
        </p>
        <FolderPicker
          label="Release Folder"
          value={releaseFolder}
          onChange={setReleaseFolder}
          placeholder="X:\Apps"
        />
        <div className="result-actions" style={{ marginTop: 12 }}>
          <button className="btn btn-primary" onClick={saveAndCheckUpdates}>
            Save and Check
          </button>
          {updateStatus?.readyToRestart && (
            <button className="btn btn-outline" onClick={() => RestartToApplyUpdate().catch(err => setUpdateError(String(err)))}>
              Restart
            </button>
          )}
        </div>
        {updateStatus && (
          <p className={updateStatus.readyToRestart ? 'success-text' : 'info-text'} style={{ marginTop: 12 }}>
            {updateStatus.message || (updateStatus.supported ? 'No update found.' : 'Windows executable updates are only available on Windows.')}
          </p>
        )}
        {updateError && <p className="danger-text" style={{ marginTop: 12 }}>{updateError}</p>}
      </div>
    </div>
  )
}
