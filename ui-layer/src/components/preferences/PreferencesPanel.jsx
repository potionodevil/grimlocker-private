import { useState } from 'react'
import { clsx } from 'clsx'
import { useGrimStore } from '../../store/useGrimStore'
import { Toggle } from '../ui/Toggle'
import { Button } from '../ui/Button'

const ACCENT_OPTIONS = [
  { key: 'blue',   label: 'Blue',   color: '#0055FF' },
  { key: 'indigo', label: 'Indigo', color: '#4F46E5' },
  { key: 'teal',   label: 'Teal',   color: '#00A3A3' },
  { key: 'green',  label: 'Green',  color: '#16A34A' },
  { key: 'purple', label: 'Purple', color: '#7C3AED' },
]

const SIDEBAR_WIDTH_OPTIONS = [
  { val: 192, label: 'Narrow' },
  { val: 224, label: 'Default' },
  { val: 272, label: 'Wide' },
]

const STARTUP_VIEWS = [
  { val: 'all',        label: 'All Items' },
  { val: 'passwords',  label: 'Passwords' },
  { val: 'FILE_VAULT', label: 'File Vault' },
]

const AUTO_LOCK_OPTIONS = [
  { val: 5,   label: '5 min' },
  { val: 10,  label: '10 min' },
  { val: 15,  label: '15 min' },
  { val: 30,  label: '30 min' },
  { val: 60,  label: '1 hour' },
  { val: 0,   label: 'Never' },
]

const CLIPBOARD_OPTIONS = [
  { val: 15,  label: '15 sec' },
  { val: 30,  label: '30 sec' },
  { val: 60,  label: '1 min' },
  { val: 0,   label: 'Never' },
]

  const TABS = [
  { id: 'appearance',   label: 'Appearance' },    // Aussehen: Theme, Accent, Dichte, Schrift
  { id: 'security',     label: 'Security' },      // Sicherheit: Auto-Lock, Clipboard, Lockdown
  { id: 'behavior',     label: 'Behavior' },      // Verhalten: Close-Button, Startup, Confirm
  { id: 'groups',       label: 'Groups' },        // Passwort-Gruppen verwalten
  { id: 'accessibility',label: 'Accessibility' }, // Barrierefreiheit: Motion, Kontrast
]

export function PreferencesPanel() {
  const store = useGrimStore()
  const {
    preferences,
    setTheme, setDensity, setFontSize, setAccentKey,
    setReduceMotion, setHighContrast, resetPreferences,
    setSidebarWidth, setAutoLockMinutes, setClipboardClearSeconds,
    setShowPasswordStrength, setLockdownThreshold,
    setCloseBehavior, setStartupView, setConfirmDelete,
    removePasswordGroup, renamePasswordGroup,
  } = store

  const {
    theme, density, fontSize, accentKey, reduceMotion, highContrast,
    sidebarWidth, autoLockMinutes, clipboardClearSeconds, showPasswordStrength,
    lockdownThreshold, closeBehavior, startupView, confirmDelete,
    passwordGroups,
  } = preferences

  const [activeTab, setActiveTab] = useState('appearance')
  const [renamingGroup, setRenamingGroup] = useState(null) // id
  const [renameValue, setRenameValue]     = useState('')

  return (
    <div className="flex h-full">
      {/* Tab sidebar */}
      <div className="w-44 shrink-0 border-r border-border p-3 space-y-0.5">
        <p className="px-2 text-[11px] font-semibold text-text-tertiary uppercase tracking-wider mb-2">
          Settings
        </p>
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={clsx(
              'w-full text-left px-3 h-8 rounded-md text-sm transition-fast',
              activeTab === tab.id
                ? 'bg-accent-subtle text-accent font-medium'
                : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
            )}
          >
            {tab.label}
          </button>
        ))}
        <div className="pt-4">
          <Button variant="ghost" size="sm" onClick={resetPreferences} className="w-full text-xs">
            Reset defaults
          </Button>
        </div>
      </div>

      {/* Tab content */}
      <div className="flex-1 overflow-y-auto p-8">

        {/* ── Appearance ── */}
        {activeTab === 'appearance' && (
          <div className="max-w-xl space-y-8">
            <TabHeader
              title="Appearance"
              description="Customize the visual style of Grimlocker."
            />

            <Section title="Theme">
              <div className="flex gap-3">
                {[
                  { val: 'light', label: 'Light', icon: '☀' },
                  { val: 'dark',  label: 'Dark',  icon: '◗' },
                ].map((t) => (
                  <ThemeCard
                    key={t.val}
                    val={t.val}
                    icon={t.icon}
                    label={t.label}
                    active={theme === t.val}
                    onClick={() => setTheme(t.val)}
                  />
                ))}
              </div>
            </Section>

            <Section title="Accent Color" description="Used for interactive elements and highlights.">
              <div className="flex items-center gap-2.5 flex-wrap">
                {ACCENT_OPTIONS.map((opt) => (
                  <button
                    key={opt.key}
                    onClick={() => setAccentKey(opt.key)}
                    title={opt.label}
                    className="relative w-8 h-8 rounded-full border-2 transition-all duration-150 hover:scale-110"
                    style={{
                      backgroundColor: opt.color,
                      borderColor: accentKey === opt.key ? 'var(--text-primary)' : 'transparent',
                      transform: accentKey === opt.key ? 'scale(1.15)' : undefined,
                    }}
                  >
                    {accentKey === opt.key && (
                      <span className="absolute inset-0 flex items-center justify-center text-white text-xs">✓</span>
                    )}
                  </button>
                ))}
              </div>
            </Section>

            <Section title="Information Density" description="Controls spacing throughout the UI.">
              <div className="flex gap-3">
                {[{ val: 'cozy', label: 'Cozy' }, { val: 'compact', label: 'Compact' }].map((d) => (
                  <SegmentButton
                    key={d.val}
                    active={density === d.val}
                    onClick={() => setDensity(d.val)}
                  >
                    {d.label}
                  </SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Font Size" description={`Base text size: ${fontSize}px`}>
              <div className="flex items-center gap-3 max-w-xs">
                <span className="text-xs text-text-tertiary w-3">A</span>
                <input
                  type="range" min={12} max={20} step={1}
                  value={fontSize}
                  onChange={(e) => setFontSize(Number(e.target.value))}
                  className="flex-1"
                />
                <span className="text-lg text-text-tertiary w-3">A</span>
                <span className="text-sm text-text-secondary w-10 text-right tabular-nums">{fontSize}px</span>
              </div>
            </Section>

            <Section title="Sidebar Width">
              <div className="flex gap-3">
                {SIDEBAR_WIDTH_OPTIONS.map((opt) => (
                  <SegmentButton
                    key={opt.val}
                    active={sidebarWidth === opt.val}
                    onClick={() => setSidebarWidth(opt.val)}
                  >
                    {opt.label}
                  </SegmentButton>
                ))}
              </div>
            </Section>
          </div>
        )}

        {/* ── Security ── */}
        {activeTab === 'security' && (
          <div className="max-w-xl space-y-8">
            <TabHeader
              title="Security"
              description="Manage vault security and locking behavior."
            />

            <Section title="Auto-lock" description="Automatically lock the vault after a period of inactivity.">
              <SegmentGroup
                options={AUTO_LOCK_OPTIONS}
                value={autoLockMinutes}
                onChange={setAutoLockMinutes}
              />
            </Section>

            <Section title="Clipboard Auto-clear" description="Clear copied passwords from clipboard after this time.">
              <SegmentGroup
                options={CLIPBOARD_OPTIONS}
                value={clipboardClearSeconds}
                onChange={setClipboardClearSeconds}
              />
            </Section>

            <Section title="Lockdown Threshold" description="Number of failed attempts before vault locks down.">
              <div className="flex gap-2">
                {[2, 3, 5, 10].map((n) => (
                  <SegmentButton
                    key={n}
                    active={lockdownThreshold === n}
                    onClick={() => setLockdownThreshold(n)}
                  >
                    {n}×
                  </SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Password Strength Indicator">
              <Toggle
                checked={showPasswordStrength}
                onChange={setShowPasswordStrength}
                label="Show strength dots on password entries"
              />
            </Section>
          </div>
        )}

        {/* ── Behavior ── */}
        {activeTab === 'behavior' && (
          <div className="max-w-xl space-y-8">
            <TabHeader
              title="Behavior"
              description="Configure how Grimlocker behaves on startup and close."
            />

            <Section title="Window Close Behavior" description="What happens when you press the window close button.">
              <div className="space-y-2">
                {[
                  {
                    val: 'quit',
                    label: 'Quit application',
                    detail: 'The app exits completely when closed.',
                  },
                  {
                    val: 'minimize',
                    label: 'Minimize to background',
                    detail: 'The window hides instead of closing. Reopen from the taskbar.',
                  },
                ].map((opt) => (
                  <label
                    key={opt.val}
                    className={clsx(
                      'flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-fast',
                      closeBehavior === opt.val
                        ? 'border-accent bg-accent-subtle/50'
                        : 'border-border hover:border-border-strong hover:bg-surface-subtle',
                    )}
                  >
                    <input
                      type="radio"
                      name="closeBehavior"
                      value={opt.val}
                      checked={closeBehavior === opt.val}
                      onChange={() => setCloseBehavior(opt.val)}
                      className="mt-0.5 accent-[var(--accent)]"
                    />
                    <div>
                      <p className="text-sm font-medium text-text-primary">{opt.label}</p>
                      <p className="text-xs text-text-tertiary mt-0.5">{opt.detail}</p>
                    </div>
                  </label>
                ))}
              </div>
            </Section>

            <Section title="Startup View" description="Which view opens when you unlock the vault.">
              <div className="flex gap-3 flex-wrap">
                {STARTUP_VIEWS.map((v) => (
                  <SegmentButton
                    key={v.val}
                    active={startupView === v.val}
                    onClick={() => setStartupView(v.val)}
                  >
                    {v.label}
                  </SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Confirmations">
              <Toggle
                checked={confirmDelete}
                onChange={setConfirmDelete}
                label="Confirm before deleting entries"
              />
            </Section>
          </div>
        )}

        {/* ── Groups ── */}
        {activeTab === 'groups' && (
          <div className="max-w-xl space-y-8">
            <TabHeader
              title="Password Groups"
              description="Organize your passwords into groups. Create groups from the sidebar."
            />

            {(!passwordGroups || passwordGroups.length === 0) ? (
              <div className="flex flex-col items-center justify-center py-12 text-center border border-dashed border-border rounded-xl">
                <p className="text-sm text-text-secondary font-medium">No groups yet</p>
                <p className="text-xs text-text-tertiary mt-1">
                  Click <span className="font-mono bg-surface-subtle px-1 rounded">+ New Group</span> in the sidebar under Passwords.
                </p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {passwordGroups.map((group) => (
                  <div
                    key={group.id}
                    className="flex items-center gap-3 px-3 py-2.5 rounded-lg border border-border bg-surface-base hover:bg-surface-subtle transition-fast"
                  >
                    <span
                      className="w-3 h-3 rounded-full shrink-0"
                      style={{ backgroundColor: group.color }}
                    />
                    {renamingGroup === group.id ? (
                      <input
                        type="text"
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') {
                            renamePasswordGroup(group.id, renameValue.trim() || group.label)
                            setRenamingGroup(null)
                          }
                          if (e.key === 'Escape') setRenamingGroup(null)
                        }}
                        onBlur={() => {
                          renamePasswordGroup(group.id, renameValue.trim() || group.label)
                          setRenamingGroup(null)
                        }}
                        autoFocus
                        className="flex-1 h-6 px-1.5 text-sm bg-surface-app border border-accent/50 rounded outline-none text-text-primary"
                      />
                    ) : (
                      <span className="flex-1 text-sm text-text-primary">{group.label}</span>
                    )}
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => { setRenamingGroup(group.id); setRenameValue(group.label) }}
                        className="h-6 px-2 text-xs text-text-tertiary hover:text-text-primary hover:bg-surface-subtle rounded transition-fast"
                      >
                        Rename
                      </button>
                      <button
                        onClick={() => removePasswordGroup(group.id)}
                        className="h-6 px-2 text-xs text-danger hover:bg-danger/5 rounded transition-fast"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ── Accessibility ── */}
        {activeTab === 'accessibility' && (
          <div className="max-w-xl space-y-8">
            <TabHeader
              title="Accessibility"
              description="Visual and motion preferences."
            />

            <Section title="Motion & Contrast">
              <div className="space-y-4">
                <Toggle
                  checked={reduceMotion}
                  onChange={setReduceMotion}
                  label="Reduce motion"
                  description="Minimizes animations and transitions."
                />
                <Toggle
                  checked={highContrast}
                  onChange={setHighContrast}
                  label="High contrast"
                  description="Increases text and border contrast."
                />
              </div>
            </Section>
          </div>
        )}

      </div>
    </div>
  )
}

// ── Lokale Sub-Komponenten für die Preferences-Tabs ──────────────────────────

function TabHeader({ title, description }) {
  return (
    <div className="mb-2">
      <h1 className="text-xl font-semibold text-text-primary">{title}</h1>
      {description && <p className="text-sm text-text-secondary mt-1">{description}</p>}
    </div>
  )
}

function Section({ title, description, children }) {
  return (
    <div>
      <div className="mb-3">
        <p className="text-sm font-semibold text-text-primary">{title}</p>
        {description && <p className="text-xs text-text-tertiary mt-0.5">{description}</p>}
      </div>
      {children}
    </div>
  )
}

function ThemeCard({ val, icon, label, active, onClick }) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        'flex items-center gap-2.5 px-4 h-10 rounded-lg border text-sm font-medium transition-fast',
        active
          ? 'border-accent bg-accent-subtle text-accent shadow-sm'
          : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
      )}
    >
      <span className="text-base">{icon}</span>
      {label}
    </button>
  )
}

function SegmentButton({ children, active, onClick }) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        'px-4 h-8 rounded-lg border text-sm font-medium transition-fast',
        active
          ? 'border-accent bg-accent-subtle text-accent shadow-sm'
          : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
      )}
    >
      {children}
    </button>
  )
}

function SegmentGroup({ options, value, onChange }) {
  return (
    <div className="flex gap-2 flex-wrap">
      {options.map((opt) => (
        <SegmentButton
          key={opt.val}
          active={value === opt.val}
          onClick={() => onChange(opt.val)}
        >
          {opt.label}
        </SegmentButton>
      ))}
    </div>
  )
}
