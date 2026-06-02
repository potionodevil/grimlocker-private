import { useState, useRef, useEffect } from 'react'
import { clsx } from 'clsx'
import { isDevMode } from '../../utils/devMode'
import { useGrimStore } from '../../store/useGrimStore'

// ── icons (SVG inline, keeps no external dependency) ──────────────────────────
function Icon({ path, size = 14, className = '' }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor"
      strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round"
      className={clsx('shrink-0', className)}
    >
      <path d={path} />
    </svg>
  )
}

const ICONS = {
  all:       'M3 12h18M3 6h18M3 18h18',
  passwords: 'M12 17v-6m0 0V9m0 2H9m3 0h3M5 20h14a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2z',
  ssh:       'M8 9l3 3-3 3M13 15h3M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z',
  certs:     'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0 1 12 2.944a11.955 11.955 0 0 1-8.618 3.04A12.02 12.02 0 0 0 3 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
  fileVault: 'M5 19a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h4l2 2h4a2 2 0 0 1 2 2v1M5 19h14a2 2 0 0 0 2-2v-5a2 2 0 0 0-2-2H9a2 2 0 0 0-2 2v5a2 2 0 0 0 2 2z',
  folder:    'M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z',
  folderOpen:'M5 19a2 2 0 0 1-2-2V7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v1M5 19h14a1 1 0 0 0 .93-.636l2.4-6A1 1 0 0 0 21.4 11H8.6a1 1 0 0 0-.93.636L5.07 18.364A1 1 0 0 0 5 19h.001z',
  admin:     'M12 4.354a4 4 0 1 1 0 5.292M15 21H3v-1a6 6 0 0 1 12 0v1zm0 0h6v-1a6 6 0 0 0-9-5.197',
  debug:     'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z',
  prefs:     'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065zM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z',
  chevron:   'M9 18l6-6-6-6',
  plus:      'M12 5v14M5 12h14',
  tag:       'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 0 1 0 2.828l-7 7a2 2 0 0 1-2.828 0l-7-7A2 2 0 0 1 3 12V7a4 4 0 0 1 4-4z',
  lock:      'M12 15v2m-6 4h12a2 2 0 0 0 2-2v-6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2zm10-10V7a4 4 0 0 0-8 0v4h8z',
  sync:      'M4 4v5h.582m15.356 2A8.001 8.001 0 0 0 4.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 0 1-15.357-2m15.357 2H15',
}

// ── group color dots ──────────────────────────────────────────────────────────
const GROUP_COLORS = [
  '#0055FF', '#7C3AED', '#DC2626', '#D97706', '#16A34A', '#0891B2',
]

// ── inline rename input ───────────────────────────────────────────────────────
function RenameInput({ value, onChange, onCommit, onCancel }) {
  const ref = useRef(null)
  useEffect(() => { ref.current?.focus(); ref.current?.select() }, [])
  return (
    <input
      ref={ref}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === 'Enter') onCommit()
        if (e.key === 'Escape') onCancel()
      }}
      onBlur={onCommit}
      className="flex-1 h-6 px-1.5 text-xs bg-surface-app border border-accent/50 rounded outline-none text-text-primary"
      onClick={(e) => e.stopPropagation()}
    />
  )
}

// ── nav item ──────────────────────────────────────────────────────────────────
function NavItem({ id, label, icon, active, onClick, children, indent = 0, badge }) {
  return (
    <div>
      <button
        onClick={() => onClick(id)}
        className={clsx(
          'w-full flex items-center gap-2 h-8 rounded-md text-sm transition-fast',
          indent === 0 ? 'px-2' : 'pl-6 pr-2',
          active
            ? 'bg-accent-subtle text-accent font-medium border-l-2 border-accent pl-[6px]'
            : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
        )}
      >
        {icon && <Icon path={icon} size={14} />}
        <span className="flex-1 text-left truncate">{label}</span>
        {badge != null && (
          <span className="text-[10px] text-text-tertiary tabular-nums">{badge}</span>
        )}
      </button>
      {children}
    </div>
  )
}

// ── section header ─────────────────────────────────────────────────────────────
function SectionHeader({ label, expanded, onToggle, onAdd, addTitle }) {
  return (
    <div className="flex items-center gap-1 px-2 mb-0.5 group">
      <button
        onClick={onToggle}
        className="flex items-center gap-1 flex-1 min-w-0"
      >
        <span className={clsx('transition-transform duration-150', expanded ? 'rotate-90' : 'rotate-0')}>
          <Icon path={ICONS.chevron} size={11} className="text-text-disabled" />
        </span>
        <span className="text-[11px] font-semibold text-text-tertiary uppercase tracking-wider truncate">
          {label}
        </span>
      </button>
      {onAdd && (
        <button
          onClick={onAdd}
          title={addTitle}
          className="opacity-0 group-hover:opacity-100 transition-opacity p-0.5 rounded hover:bg-surface-subtle text-text-tertiary hover:text-text-primary"
        >
          <Icon path={ICONS.plus} size={12} />
        </button>
      )}
    </div>
  )
}

// ── main sidebar ──────────────────────────────────────────────────────────────
export function Sidebar({
  activeView,
  onNavigate,
  // File vault
  fileVaultFolders = [],
  activeFileVaultFolder = '',
  onFileVaultFolder,
  // Password groups
  activePasswordGroup = null,
  onPasswordGroup,
  onCreatePasswordGroup,
}) {
  const { preferences } = useGrimStore()
  const passwordGroups = preferences.passwordGroups || []

  const [sectionsOpen, setSectionsOpen] = useState({
    vault: true,
    workspace: true,
    dev: false,
  })
  const [fileVaultOpen, setFileVaultOpen] = useState(true)
  const [passwordsOpen, setPasswordsOpen] = useState(true)

  const toggle = (key) => setSectionsOpen(s => ({ ...s, [key]: !s[key] }))

  // Count entries by category for badges
  const entries = useGrimStore((s) => s.entries)
  const counts = {
    all:       entries.filter(e => (e.category || e.type?.toUpperCase()) !== 'FILE_VAULT').length,
    passwords: entries.filter(e => (e.category || e.type?.toUpperCase()) === 'PASSWORD').length,
    ssh:       entries.filter(e => (e.category || e.type?.toUpperCase()) === 'SSH_KEY').length,
    certs:     entries.filter(e => (e.category || e.type?.toUpperCase()) === 'CERTIFICATE').length,
  }

  const isVaultSection = ['passwords', 'ssh', 'certs', 'FILE_VAULT'].includes(activeView)

  return (
    <aside
      className="shrink-0 flex flex-col bg-surface-app border-r border-border h-full"
      style={{ width: preferences.sidebarWidth ?? 224 }}
    >
      {/* Logo */}
      <div className="h-14 flex items-center gap-2.5 px-4 border-b border-border shrink-0">
        <div className="w-7 h-7 rounded-md bg-accent flex items-center justify-center shrink-0">
          <Icon path={ICONS.lock} size={14} className="text-white" />
        </div>
        <span className="text-base font-semibold text-text-primary tracking-tight">Grimlocker</span>
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto py-2.5 px-2 space-y-0.5">

        {/* All Items */}
        <div className="mb-1">
          <NavItem
            id="all"
            label="All Items"
            icon={ICONS.all}
            active={activeView === 'all'}
            onClick={onNavigate}
            badge={counts.all > 0 ? counts.all : undefined}
          />
        </div>

        {/* Vault section */}
        <div className="mb-1">
          <SectionHeader
            label="Vault"
            expanded={sectionsOpen.vault}
            onToggle={() => toggle('vault')}
          />
          {sectionsOpen.vault && (
            <div className="space-y-0.5">

              {/* Passwords + groups */}
              <div>
                <button
                  onClick={() => { onNavigate('passwords'); onPasswordGroup?.(null) }}
                  className={clsx(
                    'w-full flex items-center gap-2 px-2 h-8 rounded-md text-sm transition-fast',
                    activeView === 'passwords' && !activePasswordGroup
                      ? 'bg-accent-subtle text-accent font-medium border-l-2 border-accent pl-[6px]'
                      : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
                  )}
                >
                  <Icon path={ICONS.passwords} size={14} />
                  <span className="flex-1 text-left">Passwords</span>
                  {counts.passwords > 0 && (
                    <span className="text-[10px] text-text-tertiary tabular-nums">{counts.passwords}</span>
                  )}
                  <button
                    onClick={(e) => { e.stopPropagation(); setPasswordsOpen(v => !v) }}
                    className="p-0.5 rounded hover:bg-accent-subtle/50"
                  >
                    <Icon
                      path={ICONS.chevron}
                      size={11}
                      className={clsx('text-text-disabled transition-transform duration-150', passwordsOpen ? 'rotate-90' : 'rotate-0')}
                    />
                  </button>
                </button>

                {passwordsOpen && (
                  <div className="mt-0.5 space-y-0.5">
                    {passwordGroups.map(group => (
                      <button
                        key={group.id}
                        onClick={() => { onNavigate('passwords'); onPasswordGroup?.(group.id) }}
                        className={clsx(
                          'w-full flex items-center gap-2 pl-7 pr-2 h-7 rounded-md text-xs transition-fast',
                          activeView === 'passwords' && activePasswordGroup === group.id
                            ? 'bg-accent-subtle text-accent font-medium'
                            : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
                        )}
                      >
                        <span
                          className="w-2 h-2 rounded-full shrink-0"
                          style={{ backgroundColor: group.color || GROUP_COLORS[0] }}
                        />
                        <span className="flex-1 text-left truncate">{group.label}</span>
                      </button>
                    ))}
                    <button
                      onClick={() => onCreatePasswordGroup?.()}
                      className="w-full flex items-center gap-1.5 pl-7 pr-2 h-7 rounded-md text-xs text-text-tertiary hover:text-text-primary hover:bg-surface-subtle transition-fast"
                    >
                      <Icon path={ICONS.plus} size={11} />
                      New Group
                    </button>
                  </div>
                )}
              </div>

              {/* SSH */}
              <NavItem
                id="ssh"
                label="SSH Keys"
                icon={ICONS.ssh}
                active={activeView === 'ssh'}
                onClick={onNavigate}
                badge={counts.ssh > 0 ? counts.ssh : undefined}
              />

              {/* Certificates */}
              <NavItem
                id="certs"
                label="Certificates"
                icon={ICONS.certs}
                active={activeView === 'certs'}
                onClick={onNavigate}
                badge={counts.certs > 0 ? counts.certs : undefined}
              />

              {/* File Vault + folder tree */}
              <div>
                <button
                  onClick={() => { onNavigate('FILE_VAULT'); onFileVaultFolder?.('', 'Root') }}
                  className={clsx(
                    'w-full flex items-center gap-2 px-2 h-8 rounded-md text-sm transition-fast',
                    activeView === 'FILE_VAULT' && !activeFileVaultFolder
                      ? 'bg-accent-subtle text-accent font-medium border-l-2 border-accent pl-[6px]'
                      : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
                  )}
                >
                  <Icon path={ICONS.fileVault} size={14} />
                  <span className="flex-1 text-left">File Vault</span>
                  {fileVaultFolders.length > 0 && (
                    <button
                      onClick={(e) => { e.stopPropagation(); setFileVaultOpen(v => !v) }}
                      className="p-0.5 rounded hover:bg-accent-subtle/50"
                    >
                      <Icon
                        path={ICONS.chevron}
                        size={11}
                        className={clsx('text-text-disabled transition-transform duration-150', fileVaultOpen ? 'rotate-90' : 'rotate-0')}
                      />
                    </button>
                  )}
                </button>

                {fileVaultOpen && fileVaultFolders.length > 0 && (
                  <div className="mt-0.5 space-y-0.5">
                    {fileVaultFolders.map(folder => (
                      <button
                        key={folder.id}
                        onClick={() => { onNavigate('FILE_VAULT'); onFileVaultFolder?.(folder.id, folder.name) }}
                        className={clsx(
                          'w-full flex items-center gap-2 pl-7 pr-2 h-7 rounded-md text-xs transition-fast',
                          activeView === 'FILE_VAULT' && activeFileVaultFolder === folder.id
                            ? 'bg-accent-subtle text-accent font-medium'
                            : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
                        )}
                      >
                        <Icon path={ICONS.folder} size={12} />
                        <span className="flex-1 text-left truncate">{folder.name}</span>
                      </button>
                    ))}
                  </div>
                )}
              </div>

            </div>
          )}
        </div>

        {/* Workspace section */}
        <div className="mb-1">
          <SectionHeader
            label="Workspace"
            expanded={sectionsOpen.workspace}
            onToggle={() => toggle('workspace')}
          />
          {sectionsOpen.workspace && (
            <div className="space-y-0.5">
              <NavItem
                id="admin"
                label="Admin"
                icon={ICONS.admin}
                active={activeView === 'admin'}
                onClick={onNavigate}
              />
              <NavItem
                id="sync"
                label="LAN Sync"
                icon={ICONS.sync}
                active={activeView === 'sync'}
                onClick={onNavigate}
              />
            </div>
          )}
        </div>

        {/* Dev section */}
        {isDevMode() && (
          <div className="mb-1">
            <SectionHeader
              label="Development"
              expanded={sectionsOpen.dev}
              onToggle={() => toggle('dev')}
            />
            {sectionsOpen.dev && (
              <NavItem
                id="debug"
                label="Debug"
                icon={ICONS.debug}
                active={activeView === 'debug'}
                onClick={onNavigate}
              />
            )}
          </div>
        )}
      </nav>

      {/* Bottom: Preferences */}
      <div className="shrink-0 border-t border-border p-2">
        <button
          onClick={() => onNavigate('preferences')}
          className={clsx(
            'w-full flex items-center gap-2 px-2 h-8 rounded-md text-sm transition-fast',
            activeView === 'preferences'
              ? 'bg-accent-subtle text-accent font-medium'
              : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
          )}
        >
          <Icon path={ICONS.prefs} size={14} />
          Preferences
        </button>
      </div>
    </aside>
  )
}
