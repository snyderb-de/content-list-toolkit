import { ThemeMode } from '../App'
import { Screen } from '../types'

interface NavItemProps {
  id: Screen
  label: string
  icon: React.ReactNode
  active: boolean
  onClick: (s: Screen) => void
}

function NavItem({ id, label, icon, active, onClick }: NavItemProps) {
  return (
    <div className={`nav-item ${active ? 'active' : ''}`} onClick={() => onClick(id)}>
      <span className="nav-icon">{icon}</span>
      <span>{label}</span>
    </div>
  )
}

const Icons = {
  list: (
    <svg viewBox="0 0 16 16" fill="currentColor">
      <rect x="1" y="3" width="14" height="1.5" rx="0.75"/>
      <rect x="1" y="7.25" width="14" height="1.5" rx="0.75"/>
      <rect x="1" y="11.5" width="14" height="1.5" rx="0.75"/>
    </svg>
  ),
  mail: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <rect x="1" y="3" width="14" height="10" rx="1.5"/>
      <path d="M1 4.5l7 4.5 7-4.5" strokeLinejoin="round"/>
    </svg>
  ),
  clone: (
    <svg viewBox="0 0 16 16" fill="currentColor">
      <rect x="1" y="5" width="7" height="9" rx="1" opacity="0.5"/>
      <rect x="8" y="2" width="7" height="9" rx="1"/>
    </svg>
  ),
  info: (
    <svg viewBox="0 0 16 16" fill="currentColor">
      <circle cx="8" cy="8" r="7" opacity="0.15"/>
      <circle cx="8" cy="8" r="6.25" fill="none" stroke="currentColor" strokeWidth="1.5"/>
      <circle cx="8" cy="5.5" r="1"/>
      <rect x="7.25" y="7.5" width="1.5" height="4" rx="0.75"/>
    </svg>
  ),
  manual: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.35" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 2.5h7.5A2.5 2.5 0 0113 5v8.5H5.2A2.2 2.2 0 013 11.3V2.5z"/>
      <path d="M5.2 13.5A2.2 2.2 0 013 11.3V4.7A2.2 2.2 0 015.2 2.5"/>
      <path d="M5.8 6h4.4M5.8 8.4h3.6"/>
    </svg>
  ),
  sun: (
    <svg viewBox="0 0 16 16" fill="currentColor">
      <circle cx="8" cy="8" r="3"/>
      <path d="M8 1v2M8 13v2M1 8h2M13 8h2M3.05 3.05l1.41 1.41M11.54 11.54l1.41 1.41M3.05 12.95l1.41-1.41M11.54 4.46l1.41-1.41" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
    </svg>
  ),
  moon: (
    <svg viewBox="0 0 16 16" fill="currentColor">
      <path d="M11 9.5A4.5 4.5 0 016.5 5c0-.97.3-1.87.82-2.6A5.5 5.5 0 1013.1 8.7 4.5 4.5 0 0111 9.5z"/>
    </svg>
  ),
  monitor: (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
      <rect x="1" y="2" width="14" height="9" rx="1.5"/>
      <path d="M5 14h6M8 11v3"/>
    </svg>
  ),
}

const THEME_META: Record<ThemeMode, { icon: React.ReactNode; label: string }> = {
  light:  { icon: Icons.sun,     label: 'Light Mode' },
  dark:   { icon: Icons.moon,    label: 'Dark Mode' },
  system: { icon: Icons.monitor, label: 'System Mode' },
}

interface SidebarProps {
  active: Screen
  onNav: (s: Screen) => void
  theme: ThemeMode
  onCycleTheme: () => void
}

export default function Sidebar({ active, onNav, theme, onCycleTheme }: SidebarProps) {
  const { icon, label } = THEME_META[theme]

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-logo">
          Content List Toolkit
          <span>Folder scan &amp; compare tool</span>
        </div>
      </div>

      <nav className="sidebar-nav">
        <NavItem id="content-list"  label="Content List"  icon={Icons.list}  active={active === 'content-list'}  onClick={onNav} />
        <NavItem id="email-copy"    label="Email Copy"    icon={Icons.mail}  active={active === 'email-copy'}    onClick={onNav} />
        <NavItem id="clone-compare" label="Clone Compare" icon={Icons.clone} active={active === 'clone-compare'} onClick={onNav} />
        <NavItem id="manual"        label="User Manual"   icon={Icons.manual} active={active === 'manual'}        onClick={onNav} />
        <NavItem id="about"         label="About"         icon={Icons.info}  active={active === 'about'}         onClick={onNav} />
      </nav>

      <div className="sidebar-footer">
        <button className="theme-toggle-btn" onClick={onCycleTheme} title="Cycle theme">
          <span className="theme-toggle-icon">{icon}</span>
          <span>{label}</span>
        </button>
      </div>
    </aside>
  )
}
