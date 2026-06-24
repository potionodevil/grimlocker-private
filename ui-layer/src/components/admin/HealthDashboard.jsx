import { useState, useEffect } from 'react'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'

function ScoreRing({ score }) {
  const r = 36
  const circ = 2 * Math.PI * r
  const pct = score / 100
  const color = score >= 80 ? '#22c55e' : score >= 60 ? '#f59e0b' : '#ef4444'
  const label = score >= 80 ? 'Gut' : score >= 60 ? 'Mittel' : 'Schwach'

  return (
    <div className="flex flex-col items-center gap-2">
      <div className="relative w-24 h-24">
        <svg width="96" height="96" viewBox="0 0 96 96">
          <circle cx="48" cy="48" r={r} fill="none" stroke="currentColor" strokeWidth="7" className="text-border" />
          <circle
            cx="48" cy="48" r={r}
            fill="none" stroke={color} strokeWidth="7"
            strokeDasharray={`${circ} ${circ}`}
            strokeDashoffset={circ * (1 - pct)}
            strokeLinecap="round"
            transform="rotate(-90 48 48)"
            style={{ transition: 'stroke-dashoffset 1s ease, stroke 0.3s' }}
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className="text-2xl font-bold" style={{ color }}>{score}</span>
          <span className="text-[10px] text-text-tertiary">/ 100</span>
        </div>
      </div>
      <span className="text-sm font-medium" style={{ color }}>{label}</span>
    </div>
  )
}

function IssueSection({ title, items, icon, color }) {
  if (!items?.length) return null
  return (
    <div className="bg-surface-base border border-border rounded-xl overflow-hidden">
      <div className={`px-4 py-3 flex items-center gap-2 border-b border-border bg-${color}/5`}>
        <span className="text-base">{icon}</span>
        <span className="text-sm font-semibold text-text-primary">{title}</span>
        <span className={`ml-auto text-xs font-mono font-medium text-${color}`}>{items.length}</span>
      </div>
      <ul className="divide-y divide-border">
        {items.map((entry) => (
          <li key={entry.id} className="px-4 py-2.5 flex items-center justify-between gap-4">
            <div>
              <p className="text-sm text-text-primary truncate max-w-64">{entry.title || entry.id}</p>
              <p className="text-xs text-text-tertiary capitalize">{entry.category || entry.issue}</p>
            </div>
            <SeverityBadge sev={entry.severity} />
          </li>
        ))}
      </ul>
    </div>
  )
}

function SeverityBadge({ sev }) {
  if (!sev) return null
  const styles = {
    1: 'bg-text-tertiary/10 text-text-tertiary',
    2: 'bg-warning/10 text-warning',
    3: 'bg-danger/10 text-danger',
  }
  const labels = { 1: 'Info', 2: 'Warnung', 3: 'Kritisch' }
  return (
    <span className={`text-[10px] font-medium px-2 py-0.5 rounded-full shrink-0 ${styles[sev] ?? styles[1]}`}>
      {labels[sev] ?? 'Info'}
    </span>
  )
}

export function HealthDashboard() {
  const daemonStatus = useGrimStore((s) => s.daemonStatus)
  const [result, setResult] = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)

  const analyze = async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await tauriBridge.analyzeHealth()
      setResult(res)
    } catch (e) {
      setError(e?.message ?? 'Analyse fehlgeschlagen.')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (daemonStatus === 'online') analyze()
  }, [daemonStatus])

  return (
    <div className="p-6 space-y-6 max-w-2xl">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-lg font-semibold text-text-primary">Passwort-Gesundheit</h1>
          <p className="text-xs text-text-tertiary mt-0.5">
            Analyse aller Vault-Einträge auf schwache, doppelte und alte Passwörter.
          </p>
        </div>
        <button
          onClick={analyze}
          disabled={loading || daemonStatus !== 'online'}
          className="h-8 px-4 rounded-lg text-xs font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center gap-1.5"
        >
          {loading ? (
            <>
              <span className="w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" />
              Analysiere…
            </>
          ) : (
            <>
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                <polyline points="1 4 1 10 7 10"/><path d="M1 10a9 9 0 1 0 9-9"/>
              </svg>
              Neu analysieren
            </>
          )}
        </button>
      </div>

      {daemonStatus !== 'online' && (
        <div className="px-3 py-2 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs">
          Daemon nicht verbunden.
        </div>
      )}

      {error && (
        <div className="px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">
          {error}
        </div>
      )}

      {result && (
        <>
          {/* Overview */}
          <div className="bg-surface-base border border-border rounded-xl p-5 flex items-center gap-8">
            <ScoreRing score={result.score} />
            <div className="space-y-2">
              <StatRow label="Einträge analysiert" value={result.analyzed} />
              <StatRow label="Einträge gesamt" value={result.total} />
              <StatRow label="Schwache Passwörter" value={result.weak?.length ?? 0} danger={result.weak?.length > 0} />
              <StatRow label="Doppelte Passwörter" value={result.reused?.length ?? 0} danger={result.reused?.length > 0} />
              <StatRow label="Veraltete Passwörter" value={result.old?.length ?? 0} warn={result.old?.length > 0} />
            </div>
          </div>

          {/* Issue lists */}
          <IssueSection title="Schwache Passwörter" items={result.weak} icon="⚠️" color="danger" />
          <IssueSection title="Doppelte Passwörter" items={result.reused} icon="🔁" color="warning" />
          <IssueSection title="Alte Passwörter (>90 Tage)" items={result.old} icon="🕐" color="text-tertiary" />

          {!result.weak?.length && !result.reused?.length && !result.old?.length && (
            <div className="flex flex-col items-center gap-2 py-8 text-center">
              <span className="text-4xl">✅</span>
              <p className="text-sm font-medium text-text-primary">Alles in Ordnung!</p>
              <p className="text-xs text-text-tertiary">Keine Passwort-Probleme gefunden.</p>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function StatRow({ label, value, danger, warn }) {
  const cls = danger ? 'text-danger font-semibold' : warn ? 'text-warning font-semibold' : 'text-text-primary'
  return (
    <div className="flex items-center gap-8 text-xs">
      <span className="text-text-tertiary w-44">{label}</span>
      <span className={cls}>{value}</span>
    </div>
  )
}
