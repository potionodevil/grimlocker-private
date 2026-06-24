import { useState } from 'react'
import { clsx } from 'clsx'
import { useGrimStore } from '../../store/useGrimStore'
import { DESIGN_PRESETS } from '../../store/preferencesSlice'
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
  { val: 192, label: 'Schmal' },
  { val: 224, label: 'Standard' },
  { val: 272, label: 'Breit' },
]

const SIDEBAR_FONT_SIZES = [11, 12, 13, 14, 15, 16]

const STARTUP_VIEWS = [
  { val: 'all',        label: 'All Items' },
  { val: 'passwords',  label: 'Passwords' },
  { val: 'FILE_VAULT', label: 'File Vault' },
]

const AUTO_LOCK_OPTIONS = [
  { val: 5,  label: '5 min' },
  { val: 10, label: '10 min' },
  { val: 15, label: '15 min' },
  { val: 30, label: '30 min' },
  { val: 60, label: '1 Std.' },
  { val: 0,  label: 'Nie' },
]

const CLIPBOARD_OPTIONS = [
  { val: 15, label: '15 Sek.' },
  { val: 30, label: '30 Sek.' },
  { val: 60, label: '1 Min.' },
  { val: 0,  label: 'Nie' },
]

const SORT_OPTIONS = [
  { val: 'name',     label: 'Name' },
  { val: 'updated',  label: 'Geändert' },
  { val: 'created',  label: 'Erstellt' },
  { val: 'type',     label: 'Typ' },
  { val: 'strength', label: 'Stärke' },
]

const TABS = [
  { id: 'preset',        label: 'Design', icon: 'M4 16l4.586-4.586a2 2 0 0 1 2.828 0L16 16m-2-2 1.586-1.586a2 2 0 0 1 2.828 0L20 14m-6-6h.01M6 20h12a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2z' },
  { id: 'appearance',    label: 'Darstellung', icon: 'M7 21a4 4 0 0 1-4-4V5a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v12a4 4 0 0 1-4 4zm0 0h12a2 2 0 0 0 2-2v-4a2 2 0 0 0-2-2h-2.343M11 7.343l1.657-1.657a2 2 0 0 1 2.828 0l2.829 2.829a2 2 0 0 1 0 2.828l-8.486 8.485M7 17h.01' },
  { id: 'vault',         label: 'Vault-Ansicht', icon: 'M4 6h16M4 10h16M4 14h16M4 18h16' },
  { id: 'security',      label: 'Sicherheit', icon: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0 1 12 2.944a11.955 11.955 0 0 1-8.618 3.04A12.02 12.02 0 0 0 3 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z' },
  { id: 'behavior',      label: 'Verhalten', icon: 'M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 0 0 2.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 0 0 1.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 0 0-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 0 0-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 0 0-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 0 0-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 0 0 1.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065zM15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z' },
  { id: 'groups',        label: 'Gruppen', icon: 'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 0 1 0 2.828l-7 7a2 2 0 0 1-2.828 0l-7-7A2 2 0 0 1 3 12V7a4 4 0 0 1 4-4z' },
  { id: 'accessibility', label: 'Barrierefreiheit', icon: 'M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0zm-3-9a9 9 0 1 0 0 18A9 9 0 0 0 12 3z' },
]

// ── Mini-SVG-Vorschau für jedes Preset ───────────────────────────────────────
function PresetPreview({ presetKey, def, size = 120 }) {
  const r = { meridian: 6, obsidian: 2, frost: 16, carbon: 0, sakura: 20 }[presetKey] ?? 6
  const isDark = def.theme === 'dark'

  return (
    <svg width={size} height={Math.round(size * 0.67)} viewBox="0 0 120 80" xmlns="http://www.w3.org/2000/svg" className="rounded-lg overflow-hidden">
      {/* Hintergrund */}
      <rect width="120" height="80" fill={def.bgHint} />
      {/* Sidebar */}
      <rect x="0" y="0" width="32" height="80" fill={isDark ? '#1A1A1E' : '#F0F0F2'} />
      {/* Sidebar-Items */}
      {[14, 24, 34, 44, 54].map((y, i) => (
        <rect key={i} x="6" y={y} width={i === 0 ? 20 : 16} height="5" rx={Math.min(r, 3)}
          fill={i === 0 ? def.accentHint : (isDark ? '#3A3A3E' : '#D8D8DC')} opacity={i === 0 ? 1 : 0.7} />
      ))}
      {/* Topbar */}
      <rect x="32" y="0" width="88" height="12" fill={isDark ? '#141416' : '#F8F8FA'} />
      {/* Topbar Pill */}
      <rect x="36" y="3.5" width="36" height="5" rx="2.5" fill={isDark ? '#2A2A2E' : '#E8E8EA'} />
      {/* Add-Button */}
      <rect x="104" y="3" width="12" height="6" rx={Math.min(r, 3)} fill={def.accentHint} />
      {/* Karten */}
      {[[36, 16, 36, 28], [76, 16, 36, 28], [36, 48, 36, 28], [76, 48, 36, 28]].map(([x, y, w, h], i) => (
        <g key={i}>
          <rect x={x} y={y} width={w} height={h} rx={Math.min(r, 8)}
            fill={def.cardHint} stroke={isDark ? '#2E2E32' : '#E8E8EA'} strokeWidth="0.8" />
          <rect x={x + 4} y={y + 5} width="8" height="8" rx={Math.min(r, 4)}
            fill={def.accentHint} opacity="0.25" />
          <rect x={x + 14} y={y + 6} width={w - 20} height="3" rx="1.5"
            fill={isDark ? '#444' : '#CCC'} />
          <rect x={x + 14} y={y + 12} width={w - 26} height="2" rx="1"
            fill={isDark ? '#333' : '#DDD'} />
          {/* Stärke-Balken */}
          <rect x={x + 4} y={y + h - 5} width={w - 8} height="2" rx="1"
            fill={isDark ? '#2A2A2E' : '#EEE'} />
          <rect x={x + 4} y={y + h - 5} width={(w - 8) * (0.4 + i * 0.18)} height="2" rx="1"
            fill={def.accentHint} opacity="0.8" />
        </g>
      ))}
    </svg>
  )
}

export function PreferencesPanel() {
  const store = useGrimStore()
  const {
    preferences,
    setTheme, setDensity, setFontSize, setAccentKey,
    setReduceMotion, setHighContrast, resetPreferences,
    setSidebarWidth, setSidebarFontSize, setSidebarPosition,
    setAutoLockMinutes, setClipboardClearSeconds,
    setShowPasswordStrength, setLockdownThreshold,
    setCloseBehavior, setStartupView, setConfirmDelete,
    removePasswordGroup, renamePasswordGroup,
    setDesignPreset,
    setVaultViewMode, setVaultSortBy, setVaultSortDir,
    setGridCardSize, setShowEntrySubtitle, setShowEntryTimestamp,
  } = store

  const {
    theme, density, fontSize, accentKey, reduceMotion, highContrast,
    sidebarWidth, sidebarFontSize, sidebarPosition,
    autoLockMinutes, clipboardClearSeconds, showPasswordStrength,
    lockdownThreshold, closeBehavior, startupView, confirmDelete,
    passwordGroups, designPreset,
    vaultViewMode, vaultSortBy, vaultSortDir, gridCardSize,
    showEntrySubtitle, showEntryTimestamp,
  } = preferences

  const [activeTab, setActiveTab] = useState('preset')
  const [renamingGroup, setRenamingGroup] = useState(null)
  const [renameValue, setRenameValue]     = useState('')

  return (
    <div className="flex h-full">
      {/* Tab-Sidebar */}
      <div className="w-48 shrink-0 border-r border-border p-3 space-y-0.5">
        <p className="px-2 text-[11px] font-semibold text-text-tertiary uppercase tracking-wider mb-2">
          Einstellungen
        </p>
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={clsx(
              'w-full text-left px-3 h-8 rounded-md text-sm transition-fast flex items-center gap-2',
              activeTab === tab.id
                ? 'bg-accent-subtle text-accent font-medium'
                : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
            )}
          >
            <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" className="shrink-0 opacity-70">
              <path d={tab.icon}/>
            </svg>
            {tab.label}
          </button>
        ))}
        <div className="pt-4">
          <Button variant="ghost" size="sm" onClick={resetPreferences} className="w-full text-xs">
            Zurücksetzen
          </Button>
        </div>
      </div>

      {/* Tab-Inhalt */}
      <div className="flex-1 overflow-y-auto p-8">

        {/* ── Design-Presets ── */}
        {activeTab === 'preset' && (
          <div className="max-w-2xl space-y-8">
            <TabHeader
              title="Design-Preset"
              description="Wähle eine komplette UI-Ästhetik. Wird beim nächsten App-Start aktiv."
            />
            <div className="grid grid-cols-2 gap-4 xl:grid-cols-3">
              {Object.entries(DESIGN_PRESETS).map(([key, def]) => {
                const isActive = designPreset === key
                return (
                  <button
                    key={key}
                    onClick={() => !isActive && setDesignPreset(key)}
                    className={clsx(
                      'flex flex-col items-start gap-3 p-4 rounded-xl border-2 text-left transition-base',
                      isActive
                        ? 'border-accent bg-accent-subtle/40 shadow-sm ring-1 ring-accent/30'
                        : 'border-border bg-surface-base hover:border-accent/40 hover:shadow-sm cursor-pointer',
                    )}
                  >
                    <PresetPreview presetKey={key} def={def} />
                    <div className="w-full">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-sm font-semibold text-text-primary">{def.label}</span>
                        {isActive ? (
                          <span className="flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded-full bg-accent text-white">
                            <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={3}><path d="M5 13l4 4L19 7"/></svg>
                            Aktiv
                          </span>
                        ) : (
                          <span className="text-[10px] text-text-tertiary">{def.theme === 'dark' ? '◗ Dunkel' : '☀ Hell'}</span>
                        )}
                      </div>
                      <p className="text-xs text-text-tertiary mt-0.5">{def.description}</p>
                    </div>
                  </button>
                )
              })}
            </div>
            <div className="rounded-lg border border-border bg-surface-subtle p-4 text-sm text-text-secondary leading-relaxed">
              <strong className="text-text-primary">Sofort aktiv:</strong> Presets wechseln Theme, Farben, Schrift und Radius — direkt beim Klick, kein Neustart nötig.
            </div>
          </div>
        )}

        {/* ── Darstellung ── */}
        {activeTab === 'appearance' && (
          <div className="max-w-xl space-y-8">
            <TabHeader title="Darstellung" description="Passe das visuelle Erscheinungsbild an." />

            <Section title="Theme">
              <div className="flex gap-3">
                {[
                  { val: 'light', label: 'Hell', icon: '☀' },
                  { val: 'dark',  label: 'Dunkel', icon: '◗' },
                ].map((t) => (
                  <ThemeCard key={t.val} val={t.val} icon={t.icon} label={t.label}
                    active={theme === t.val} onClick={() => setTheme(t.val)} />
                ))}
              </div>
            </Section>

            <Section title="Akzentfarbe" description="Wird für interaktive Elemente und Hervorhebungen verwendet.">
              <div className="flex items-center gap-3 flex-wrap">
                {ACCENT_OPTIONS.map((opt) => (
                  <button
                    key={opt.key}
                    onClick={() => setAccentKey(opt.key)}
                    title={opt.label}
                    className="relative w-9 h-9 rounded-full border-2 transition-all duration-150"
                    style={{
                      backgroundColor: opt.color,
                      borderColor: accentKey === opt.key ? 'var(--text-primary)' : 'transparent',
                      transform: accentKey === opt.key ? 'scale(1.18)' : undefined,
                    }}
                  >
                    {accentKey === opt.key && (
                      <span className="absolute inset-0 flex items-center justify-center text-white text-sm">✓</span>
                    )}
                  </button>
                ))}
              </div>
            </Section>

            <Section title="Informationsdichte" description="Bestimmt den Abstand zwischen Elementen.">
              <div className="flex gap-3">
                {[{ val: 'cozy', label: 'Bequem' }, { val: 'compact', label: 'Kompakt' }].map((d) => (
                  <SegmentButton key={d.val} active={density === d.val} onClick={() => setDensity(d.val)}>{d.label}</SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Basis-Schriftgrösse" description={`Globale Textgrösse: ${fontSize}px`}>
              <div className="flex items-center gap-3 max-w-xs">
                <span className="text-xs text-text-tertiary">A</span>
                <input type="range" min={12} max={20} step={1} value={fontSize}
                  onChange={(e) => setFontSize(Number(e.target.value))} className="flex-1" />
                <span className="text-lg text-text-tertiary">A</span>
                <span className="text-sm text-text-secondary w-10 text-right tabular-nums">{fontSize}px</span>
              </div>
            </Section>

            <Section title="Sidebar-Schriftgrösse" description={`Schrift in der Seitenleiste: ${sidebarFontSize ?? 13}px`}>
              <div className="flex items-center gap-1.5">
                <span className="text-[11px] text-text-disabled font-medium w-4">A</span>
                <div className="flex items-center gap-1">
                  {SIDEBAR_FONT_SIZES.map((s) => (
                    <button
                      key={s}
                      onClick={() => setSidebarFontSize(s)}
                      className={clsx(
                        'w-9 h-8 rounded-md text-xs font-mono transition-fast border',
                        (sidebarFontSize ?? 13) === s
                          ? 'bg-accent text-white border-accent'
                          : 'bg-surface-subtle text-text-secondary border-border hover:border-border-strong hover:text-text-primary',
                      )}
                    >
                      {s}
                    </button>
                  ))}
                </div>
                <span className="text-base text-text-disabled font-medium w-4">A</span>
              </div>
            </Section>

            <Section title="Sidebar-Breite">
              <div className="flex gap-3">
                {SIDEBAR_WIDTH_OPTIONS.map((opt) => (
                  <SegmentButton key={opt.val} active={sidebarWidth === opt.val} onClick={() => setSidebarWidth(opt.val)}>
                    {opt.label}
                  </SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Sidebar-Position" description="Wo die Navigationsleiste angezeigt wird.">
              <div className="flex gap-3">
                {[
                  { val: 'left',  label: '← Links',  icon: 'M11 19l-7-7 7-7m8 14l-7-7 7-7' },
                  { val: 'right', label: 'Rechts →', icon: 'M13 5l7 7-7 7M5 5l7 7-7 7' },
                ].map(({ val, label, icon }) => (
                  <button
                    key={val}
                    onClick={() => setSidebarPosition(val)}
                    className={clsx(
                      'flex items-center gap-2 px-4 h-9 rounded-lg border text-sm font-medium transition-fast',
                      (sidebarPosition ?? 'left') === val
                        ? 'border-accent bg-accent-subtle text-accent shadow-sm'
                        : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
                    )}
                  >
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                      <path d={icon}/>
                    </svg>
                    {label}
                  </button>
                ))}
              </div>
            </Section>
          </div>
        )}

        {/* ── Vault-Ansicht ── */}
        {activeTab === 'vault' && (
          <div className="max-w-xl space-y-8">
            <TabHeader title="Vault-Ansicht" description="Standard-Darstellung und Sortierung deiner Passwörter." />

            <Section title="Standard-Ansicht" description="Wie Einträge beim Öffnen dargestellt werden.">
              <div className="flex gap-2">
                {[
                  { val: 'grid',    label: 'Grid',    icon: 'M3 3h7v7H3V3zm11 0h7v7h-7V3zM3 14h7v7H3v-7zm11 0h7v7h-7v-7z' },
                  { val: 'list',    label: 'Liste',   icon: 'M3 6h18M3 12h18M3 18h18' },
                  { val: 'compact', label: 'Kompakt', icon: 'M3 5h18M3 9h18M3 13h18M3 17h18' },
                ].map(({ val, label, icon }) => (
                  <button
                    key={val}
                    onClick={() => setVaultViewMode(val)}
                    className={clsx(
                      'flex flex-col items-center gap-1.5 px-4 py-3 rounded-lg border text-xs font-medium transition-fast',
                      vaultViewMode === val
                        ? 'border-accent bg-accent-subtle text-accent'
                        : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong',
                    )}
                  >
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
                      <path d={icon}/>
                    </svg>
                    {label}
                  </button>
                ))}
              </div>
            </Section>

            <Section title="Grid-Kartengrösse" description="Breite der Karten in der Grid-Ansicht.">
              <div className="flex gap-3">
                {[
                  { val: 'small',   label: 'Klein' },
                  { val: 'default', label: 'Standard' },
                  { val: 'large',   label: 'Groß' },
                ].map((opt) => (
                  <SegmentButton key={opt.val} active={(gridCardSize ?? 'default') === opt.val} onClick={() => setGridCardSize(opt.val)}>
                    {opt.label}
                  </SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Standard-Sortierung">
              <div className="flex gap-2 flex-wrap">
                {SORT_OPTIONS.map((o) => (
                  <SegmentButton key={o.val} active={vaultSortBy === o.val} onClick={() => setVaultSortBy(o.val)}>
                    {o.label}
                  </SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Sortierrichtung">
              <div className="flex gap-3">
                <SegmentButton active={vaultSortDir === 'asc'} onClick={() => setVaultSortDir('asc')}>↑ Aufsteigend</SegmentButton>
                <SegmentButton active={vaultSortDir === 'desc'} onClick={() => setVaultSortDir('desc')}>↓ Absteigend</SegmentButton>
              </div>
            </Section>

            <Section title="Karten-Inhalte" description="Was in jeder Eintrags-Karte angezeigt wird.">
              <div className="space-y-3">
                <Toggle checked={showEntrySubtitle ?? true} onChange={setShowEntrySubtitle}
                  label="Benutzername / Subtitle anzeigen" />
                <Toggle checked={showEntryTimestamp ?? true} onChange={setShowEntryTimestamp}
                  label="Änderungsdatum anzeigen" />
                <Toggle checked={showPasswordStrength} onChange={setShowPasswordStrength}
                  label="Passwort-Stärke anzeigen" />
              </div>
            </Section>
          </div>
        )}

        {/* ── Sicherheit ── */}
        {activeTab === 'security' && (
          <div className="max-w-xl space-y-8">
            <TabHeader title="Sicherheit" description="Vault-Sperrung und Sicherheitseinstellungen." />

            <Section title="Auto-Sperrung" description="Vault automatisch nach Inaktivität sperren.">
              <SegmentGroup options={AUTO_LOCK_OPTIONS} value={autoLockMinutes} onChange={setAutoLockMinutes} />
            </Section>

            <Section title="Clipboard Auto-Clear" description="Kopierte Passwörter nach dieser Zeit aus dem Clipboard löschen.">
              <SegmentGroup options={CLIPBOARD_OPTIONS} value={clipboardClearSeconds} onChange={setClipboardClearSeconds} />
            </Section>

            <Section title="Lockdown-Schwelle" description="Anzahl Fehlversuche bis zum Lockdown.">
              <div className="flex gap-2">
                {[2, 3, 5, 10].map((n) => (
                  <SegmentButton key={n} active={lockdownThreshold === n} onClick={() => setLockdownThreshold(n)}>{n}×</SegmentButton>
                ))}
              </div>
            </Section>
          </div>
        )}

        {/* ── Verhalten ── */}
        {activeTab === 'behavior' && (
          <div className="max-w-xl space-y-8">
            <TabHeader title="Verhalten" description="Start- und Schliessverhalten des Programms." />

            <Section title="Fenster schließen" description="Was passiert beim Klick auf X.">
              <div className="space-y-2">
                {[
                  { val: 'quit',     label: 'App beenden',         detail: 'Grimlocker wird vollständig beendet.' },
                  { val: 'minimize', label: 'Im Hintergrund lassen', detail: 'Fenster wird ausgeblendet, Daemon läuft weiter.' },
                ].map((opt) => (
                  <label key={opt.val} className={clsx(
                    'flex items-start gap-3 p-3 rounded-lg border cursor-pointer transition-fast',
                    closeBehavior === opt.val
                      ? 'border-accent bg-accent-subtle/50'
                      : 'border-border hover:border-border-strong hover:bg-surface-subtle',
                  )}>
                    <input type="radio" name="closeBehavior" value={opt.val}
                      checked={closeBehavior === opt.val} onChange={() => setCloseBehavior(opt.val)}
                      className="mt-0.5 accent-[var(--accent)]" />
                    <div>
                      <p className="text-sm font-medium text-text-primary">{opt.label}</p>
                      <p className="text-xs text-text-tertiary mt-0.5">{opt.detail}</p>
                    </div>
                  </label>
                ))}
              </div>
            </Section>

            <Section title="Startansicht" description="Welche Ansicht beim Entsperren des Vaults geöffnet wird.">
              <div className="flex gap-3 flex-wrap">
                {STARTUP_VIEWS.map((v) => (
                  <SegmentButton key={v.val} active={startupView === v.val} onClick={() => setStartupView(v.val)}>{v.label}</SegmentButton>
                ))}
              </div>
            </Section>

            <Section title="Bestätigungen">
              <Toggle checked={confirmDelete} onChange={setConfirmDelete} label="Vor dem Löschen bestätigen" />
            </Section>
          </div>
        )}

        {/* ── Gruppen ── */}
        {activeTab === 'groups' && (
          <div className="max-w-xl space-y-8">
            <TabHeader title="Passwort-Gruppen" description='Gruppen anlegen über "+ New Group" in der Sidebar.' />
            {(!passwordGroups || passwordGroups.length === 0) ? (
              <div className="flex flex-col items-center justify-center py-12 text-center border border-dashed border-border rounded-xl">
                <p className="text-sm text-text-secondary font-medium">Keine Gruppen vorhanden</p>
                <p className="text-xs text-text-tertiary mt-1">Klicke auf <span className="font-mono bg-surface-subtle px-1 rounded">+ New Group</span> in der Sidebar.</p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {passwordGroups.map((group) => (
                  <div key={group.id} className="flex items-center gap-3 px-3 py-2.5 rounded-lg border border-border bg-surface-base hover:bg-surface-subtle transition-fast">
                    <span className="w-3 h-3 rounded-full shrink-0" style={{ backgroundColor: group.color }} />
                    {renamingGroup === group.id ? (
                      <input type="text" value={renameValue} onChange={(e) => setRenameValue(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') { renamePasswordGroup(group.id, renameValue.trim() || group.label); setRenamingGroup(null) }
                          if (e.key === 'Escape') setRenamingGroup(null)
                        }}
                        onBlur={() => { renamePasswordGroup(group.id, renameValue.trim() || group.label); setRenamingGroup(null) }}
                        autoFocus
                        className="flex-1 h-6 px-1.5 text-sm bg-surface-app border border-accent/50 rounded outline-none text-text-primary"
                      />
                    ) : (
                      <span className="flex-1 text-sm text-text-primary">{group.label}</span>
                    )}
                    <div className="flex items-center gap-1">
                      <button onClick={() => { setRenamingGroup(group.id); setRenameValue(group.label) }}
                        className="h-6 px-2 text-xs text-text-tertiary hover:text-text-primary hover:bg-surface-subtle rounded transition-fast">Umbenennen</button>
                      <button onClick={() => removePasswordGroup(group.id)}
                        className="h-6 px-2 text-xs text-danger hover:bg-danger/5 rounded transition-fast">Löschen</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* ── Barrierefreiheit ── */}
        {activeTab === 'accessibility' && (
          <div className="max-w-xl space-y-8">
            <TabHeader title="Barrierefreiheit" description="Bewegung und Kontrast." />
            <Section title="Bewegung & Kontrast">
              <div className="space-y-4">
                <Toggle checked={reduceMotion} onChange={setReduceMotion}
                  label="Bewegung reduzieren" description="Minimiert Animationen und Übergänge." />
                <Toggle checked={highContrast} onChange={setHighContrast}
                  label="Hoher Kontrast" description="Erhöht den Text- und Rahmenkontrast." />
              </div>
            </Section>
          </div>
        )}

      </div>
    </div>
  )
}

// ── Lokale Sub-Komponenten ────────────────────────────────────────────────────

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
    <button onClick={onClick} className={clsx(
      'flex items-center gap-2.5 px-4 h-10 rounded-lg border text-sm font-medium transition-fast',
      active
        ? 'border-accent bg-accent-subtle text-accent shadow-sm'
        : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
    )}>
      <span className="text-base">{icon}</span>
      {label}
    </button>
  )
}

function SegmentButton({ children, active, onClick }) {
  return (
    <button onClick={onClick} className={clsx(
      'px-4 h-8 rounded-lg border text-sm font-medium transition-fast',
      active
        ? 'border-accent bg-accent-subtle text-accent shadow-sm'
        : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
    )}>
      {children}
    </button>
  )
}

function SegmentGroup({ options, value, onChange }) {
  return (
    <div className="flex gap-2 flex-wrap">
      {options.map((opt) => (
        <SegmentButton key={opt.val} active={value === opt.val} onClick={() => onChange(opt.val)}>
          {opt.label}
        </SegmentButton>
      ))}
    </div>
  )
}
