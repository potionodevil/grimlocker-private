import { useState, useEffect } from 'react'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'

/**
 * VaultHealthScore — kompaktes Score-Widget (0-100) für das Dashboard.
 * Zeigt einen animierten Ring und Verbesserungs-Tips.
 * Kann in HealthCards oder als eigenständige Karte genutzt werden.
 */
export function VaultHealthScore({ compact = false }) {
  const daemonStatus = useGrimStore((s) => s.daemonStatus)
  const [score, setScore] = useState(null)
  const [issues, setIssues] = useState({ weak: 0, reused: 0, old: 0 })
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (daemonStatus !== 'online') return
    setLoading(true)
    tauriBridge.analyzeHealth()
      .then((r) => {
        setScore(r.score ?? 100)
        setIssues({
          weak:   r.weak?.length ?? 0,
          reused: r.reused?.length ?? 0,
          old:    r.old?.length ?? 0,
        })
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [daemonStatus])

  if (loading) return <ScoreSkeleton compact={compact} />
  if (score === null) return null

  return compact
    ? <CompactScore score={score} />
    : <FullScore score={score} issues={issues} />
}

function CompactScore({ score }) {
  const color = score >= 80 ? '#22c55e' : score >= 60 ? '#f59e0b' : '#ef4444'
  return (
    <div className="flex items-center gap-2">
      <SmallRing score={score} color={color} />
      <div>
        <p className="text-xs text-text-tertiary">Vault Score</p>
        <p className="text-sm font-bold" style={{ color }}>{score} / 100</p>
      </div>
    </div>
  )
}

function FullScore({ score, issues }) {
  const color = score >= 80 ? '#22c55e' : score >= 60 ? '#f59e0b' : '#ef4444'
  const label = score >= 80 ? 'Sehr sicher' : score >= 60 ? 'Verbesserbar' : 'Handlungsbedarf'

  const tips = []
  if (issues.weak > 0) tips.push(`${issues.weak} schwache${issues.weak > 1 ? '' : 's'} Passwort${issues.weak > 1 ? 'wörter' : ''} stärken`)
  if (issues.reused > 0) tips.push(`${issues.reused} doppelt${issues.reused > 1 ? 'e' : 'es'} Passwort${issues.reused > 1 ? 'wörter' : ''} ersetzen`)
  if (issues.old > 0) tips.push(`${issues.old} alte${issues.old > 1 ? '' : 's'} Passwort${issues.old > 1 ? 'wörter' : ''} aktualisieren`)

  const r = 44
  const circ = 2 * Math.PI * r
  const pct = score / 100

  return (
    <div className="bg-surface-base border border-border rounded-xl p-5 space-y-4">
      <div className="flex items-center gap-5">
        {/* Ring */}
        <div className="relative w-28 h-28 shrink-0">
          <svg width="112" height="112" viewBox="0 0 112 112">
            <circle cx="56" cy="56" r={r} fill="none" stroke="currentColor" strokeWidth="8" className="text-border" />
            <circle
              cx="56" cy="56" r={r}
              fill="none" stroke={color} strokeWidth="8"
              strokeDasharray={`${circ} ${circ}`}
              strokeDashoffset={circ * (1 - pct)}
              strokeLinecap="round"
              transform="rotate(-90 56 56)"
              style={{ transition: 'stroke-dashoffset 1.2s ease, stroke 0.4s' }}
            />
          </svg>
          <div className="absolute inset-0 flex flex-col items-center justify-center">
            <span className="text-3xl font-black" style={{ color }}>{score}</span>
            <span className="text-[10px] text-text-tertiary font-medium">/ 100</span>
          </div>
        </div>

        {/* Label + tips */}
        <div className="space-y-2">
          <div>
            <p className="text-base font-semibold" style={{ color }}>{label}</p>
            <p className="text-xs text-text-tertiary">Dein Vault-Sicherheitsscore</p>
          </div>
          {tips.length === 0 ? (
            <p className="text-xs text-green-400 flex items-center gap-1.5">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}><polyline points="20 6 9 17 4 12" /></svg>
              Alles in Ordnung!
            </p>
          ) : (
            <ul className="space-y-1">
              {tips.map((tip) => (
                <li key={tip} className="text-xs text-text-secondary flex items-start gap-1.5">
                  <span className="text-warning mt-0.5">→</span>
                  {tip}
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  )
}

function SmallRing({ score, color }) {
  const r = 12
  const circ = 2 * Math.PI * r
  return (
    <svg width="32" height="32" viewBox="0 0 32 32">
      <circle cx="16" cy="16" r={r} fill="none" stroke="currentColor" strokeWidth="3" className="text-border" />
      <circle
        cx="16" cy="16" r={r}
        fill="none" stroke={color} strokeWidth="3"
        strokeDasharray={`${circ} ${circ}`}
        strokeDashoffset={circ * (1 - score / 100)}
        strokeLinecap="round"
        transform="rotate(-90 16 16)"
        style={{ transition: 'stroke-dashoffset 1s ease' }}
      />
    </svg>
  )
}

function ScoreSkeleton({ compact }) {
  if (compact) return <div className="w-24 h-8 bg-surface-subtle rounded-md animate-pulse" />
  return <div className="bg-surface-base border border-border rounded-xl p-5 h-32 animate-pulse" />
}
