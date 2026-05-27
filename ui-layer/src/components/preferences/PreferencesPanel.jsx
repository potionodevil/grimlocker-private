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

export function PreferencesPanel() {
  const {
    preferences,
    setTheme, setDensity, setFontSize, setAccentKey,
    setReduceMotion, setHighContrast, resetPreferences,
  } = useGrimStore()

  const { theme, density, fontSize, accentKey, reduceMotion, highContrast } = preferences

  return (
    <div className="max-w-xl mx-auto p-8 space-y-8">
      <div>
        <h1 className="text-2xl font-semibold text-text-primary">Preferences</h1>
        <p className="text-sm text-text-secondary mt-1">Customize appearance and accessibility settings.</p>
      </div>

      {/* ── Appearance ── */}
      <Section title="Appearance">
        {/* Theme */}
        <Setting label="Theme" description="Choose between light and dark mode.">
          <div className="flex gap-3">
            {[{ val: 'light', icon: '☀', label: 'Light' }, { val: 'dark', icon: '◗', label: 'Dark' }].map((t) => (
              <button
                key={t.val}
                onClick={() => setTheme(t.val)}
                className={clsx(
                  'flex items-center gap-2 px-4 h-9 rounded-md border text-sm font-medium transition-fast',
                  theme === t.val
                    ? 'border-accent bg-accent-subtle text-accent'
                    : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
                )}
              >
                <span>{t.icon}</span> {t.label}
              </button>
            ))}
          </div>
        </Setting>

        {/* Accent color */}
        <Setting label="Accent Color" description="Brand color used for highlights and interactive elements.">
          <div className="flex items-center gap-2 flex-wrap">
            {ACCENT_OPTIONS.map((opt) => (
              <button
                key={opt.key}
                onClick={() => setAccentKey(opt.key)}
                title={opt.label}
                className={clsx(
                  'w-7 h-7 rounded-full border-2 transition-fast',
                  accentKey === opt.key ? 'border-text-primary scale-110' : 'border-transparent hover:scale-105',
                )}
                style={{ backgroundColor: opt.color }}
              />
            ))}
            <span className="text-sm text-text-tertiary ml-1">
              {ACCENT_OPTIONS.find((o) => o.key === accentKey)?.color}
            </span>
          </div>
        </Setting>

        {/* Density */}
        <Setting label="Information Density" description="Controls padding and spacing across all views.">
          <div className="flex gap-3">
            {[{ val: 'cozy', label: 'Cozy' }, { val: 'compact', label: 'Compact' }].map((d) => (
              <button
                key={d.val}
                onClick={() => setDensity(d.val)}
                className={clsx(
                  'flex items-center gap-2 px-4 h-9 rounded-md border text-sm font-medium transition-fast',
                  density === d.val
                    ? 'border-accent bg-accent-subtle text-accent'
                    : 'border-border bg-surface-subtle text-text-secondary hover:border-border-strong hover:text-text-primary',
                )}
              >
                {d.label}
              </button>
            ))}
          </div>
        </Setting>

        {/* Font size */}
        <Setting
          label="Base Font Size"
          description="Adjusts the base text size across the entire UI."
        >
          <div className="flex items-center gap-3">
            <span className="text-sm text-text-tertiary">A</span>
            <input
              type="range" min={12} max={20} step={1}
              value={fontSize}
              onChange={(e) => setFontSize(Number(e.target.value))}
              className="flex-1"
            />
            <span className="text-lg text-text-tertiary">A</span>
            <span className="text-sm text-text-secondary w-8 text-right tabular-nums">{fontSize}px</span>
          </div>
        </Setting>
      </Section>

      {/* ── Accessibility ── */}
      <Section title="Accessibility">
        <div className="space-y-4">
          <Toggle
            checked={reduceMotion}
            onChange={setReduceMotion}
            label="Reduce motion"
          />
          <Toggle
            checked={highContrast}
            onChange={setHighContrast}
            label="High contrast"
          />
        </div>
      </Section>

      {/* Reset */}
      <div>
        <Button variant="ghost" size="sm" onClick={resetPreferences}>
          Reset to defaults
        </Button>
      </div>
    </div>
  )
}

function Section({ title, children }) {
  return (
    <div>
      <h2 className="text-base font-semibold text-text-primary mb-4">{title}</h2>
      <div className="space-y-5">
        {children}
      </div>
    </div>
  )
}

function Setting({ label, description, children }) {
  return (
    <div className="grid grid-cols-[1fr_auto] gap-6 items-start">
      <div>
        <p className="text-sm font-medium text-text-primary">{label}</p>
        {description && <p className="text-sm text-text-tertiary mt-0.5">{description}</p>}
      </div>
      <div>{children}</div>
    </div>
  )
}
