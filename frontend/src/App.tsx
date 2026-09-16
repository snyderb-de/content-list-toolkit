import { useEffect, useState } from 'react'
import Sidebar from './components/Sidebar'
import ContentList from './screens/ContentList'
import EmailCopy from './screens/EmailCopy'
import CloneCompare from './screens/CloneCompare'
import GettyTag from './screens/GettyTag'
import UserManual from './screens/UserManual'
import About from './screens/About'
import { CheckForUpdates, RestartToApplyUpdate } from '../wailsjs/go/main/App'
import { Screen, UpdateStatus } from './types'

export type ThemeMode = 'light' | 'dark' | 'system'

const THEME_KEY = 'clg-theme'

function getSystemDark() {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function applyTheme(mode: ThemeMode) {
  const dark = mode === 'system' ? getSystemDark() : mode === 'dark'
  document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light')
}

function loadTheme(): ThemeMode {
  const saved = localStorage.getItem(THEME_KEY) as ThemeMode | null
  return saved ?? 'system'
}

export default function App() {
  const [screen, setScreen] = useState<Screen>('content-list')
  const [updateReady, setUpdateReady] = useState<UpdateStatus | null>(null)
  const [theme, setTheme] = useState<ThemeMode>(() => {
    const t = loadTheme()
    applyTheme(t)
    return t
  })

  useEffect(() => {
    if (theme !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const handler = () => applyTheme('system')
    mq.addEventListener('change', handler)
    return () => mq.removeEventListener('change', handler)
  }, [theme])

  useEffect(() => {
    CheckForUpdates()
      .then(status => {
        if (status.readyToRestart) setUpdateReady(status)
      })
      .catch(() => {})
  }, [])

  const cycleTheme = () => {
    const next: ThemeMode = theme === 'light' ? 'dark' : theme === 'dark' ? 'system' : 'light'
    setTheme(next)
    applyTheme(next)
    localStorage.setItem(THEME_KEY, next)
  }

  return (
    <div className="app-container">
      <Sidebar active={screen} onNav={setScreen} theme={theme} onCycleTheme={cycleTheme} />
      <main className="main-content">
        {updateReady && (
          <div className="update-banner" role="status">
            <div>
              <strong>Update ready</strong>
              <span>Version {updateReady.latestVersion} has been downloaded. Restart the app to finish updating.</span>
            </div>
            <button className="btn btn-primary btn-sm" onClick={() => RestartToApplyUpdate().catch(() => {})}>
              Restart
            </button>
          </div>
        )}
        {screen === 'content-list'  && <ContentList />}
        {screen === 'email-copy'    && <EmailCopy />}
        {screen === 'clone-compare' && <CloneCompare />}
        {screen === 'getty-tag'     && <GettyTag />}
        {screen === 'manual'        && <UserManual />}
        {screen === 'about'         && <About />}
      </main>
    </div>
  )
}
