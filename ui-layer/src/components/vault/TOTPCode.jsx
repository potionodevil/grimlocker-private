import { useState, useEffect, useCallback, useRef } from 'react'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * TOTPCode — Live TOTP-Code-Widget mit Countdown-Ring.
 *
 * Props:
 *   secret      — Base32-kodiertes Shared-Secret (Pflicht)
 *   issuer      — optional, für Anzeige
 *   algorithm   — SHA1 | SHA256 | SHA512 (default SHA1)
 *   digits      — 6 | 8 (default 6)
 *   period      — Sekunden (default 30)
 *   onCopy      — optional callback wenn Code kopiert
 */
export function TOTPCode({ secret, issuer, algorithm = 'SHA1', digits = 6, period = 30, onCopy }) {
  const [code, setCode]           = useState('------')
  const [expiresIn, setExpiresIn] = useState(period)
  const [copied, setCopied]       = useState(false)
  const [error, setError]         = useState(null)
  const intervalRef               = useRef(null)

  const refresh = useCallback(async () => {
    if (!secret) return
    try {
      const res = await tauriBridge.generateTOTP({ secret, issuer, algorithm, digits, period })
      setCode(res.code)
      setExpiresIn(res.expires_in)
      setError(null)
    } catch (e) {
      setError(e?.message ?? 'TOTP-Fehler')
    }
  }, [secret, issuer, algorithm, digits, period])

  useEffect(() => {
    refresh()
    // Tick-Counter — aktualisiert expiresIn sekündlich; neuen Code erst wenn period abläuft
    intervalRef.current = setInterval(() => {
      setExpiresIn(prev => {
        if (prev <= 1) {
          refresh()
          return period
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(intervalRef.current)
  }, [refresh, period])

  const handleCopy = async () => {
    if (code === '------') return
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
      onCopy?.(code)
    } catch { /* clipboard blocked */ }
  }

  // SVG-Ring-Countdown
  const radius   = 10
  const circ     = 2 * Math.PI * radius
  const progress = expiresIn / period
  // Farbe wechselt: grün → gelb (≤10s) → rot (≤5s)
  const ringColor = expiresIn <= 5 ? '#ef4444' : expiresIn <= 10 ? '#f59e0b' : '#22c55e'

  // Code in zwei Hälften aufteilen (z.B. "123 456")
  const half    = Math.ceil(digits / 2)
  const display = code === '------' ? code : `${code.slice(0, half)} ${code.slice(half)}`

  if (error) {
    return (
      <div className="flex items-center gap-2 text-xs text-danger">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
          <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
        </svg>
        {error}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-3">
      {/* Countdown-Ring */}
      <svg width="28" height="28" viewBox="0 0 28 28">
        {/* Track */}
        <circle cx="14" cy="14" r={radius} fill="none" stroke="currentColor" strokeWidth="2.5" className="text-border" />
        {/* Progress */}
        <circle
          cx="14" cy="14" r={radius}
          fill="none"
          stroke={ringColor}
          strokeWidth="2.5"
          strokeDasharray={`${circ} ${circ}`}
          strokeDashoffset={circ * (1 - progress)}
          strokeLinecap="round"
          transform="rotate(-90 14 14)"
          style={{ transition: 'stroke-dashoffset 1s linear, stroke 0.3s' }}
        />
        <text x="14" y="18" textAnchor="middle" fontSize="8" fill={ringColor} fontWeight="600">
          {expiresIn}
        </text>
      </svg>

      {/* Code */}
      <button
        onClick={handleCopy}
        title="Klicken zum Kopieren"
        className="font-mono text-xl font-bold tracking-widest text-text-primary hover:text-accent transition-fast select-all cursor-pointer"
      >
        {display}
      </button>

      {/* Copy-Feedback */}
      {copied && (
        <span className="text-xs text-green-400 animate-fade-in">Kopiert!</span>
      )}
    </div>
  )
}

/**
 * TOTPEntryCard — kompaktes TOTP-Entry für den VaultGrid.
 * Zeigt Issuer + Live-Code + Copy-Button.
 */
export function TOTPEntryCard({ entry, onClick }) {
  const fields = entry.fields ?? entry.decryptedFields ?? {}

  return (
    <div
      className="bg-surface-base border border-border rounded-xl p-4 flex flex-col gap-3 cursor-pointer hover:border-accent/50 transition-fast"
      onClick={onClick}
    >
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-semibold text-text-primary truncate">{entry.title}</p>
          {fields.account && (
            <p className="text-xs text-text-tertiary truncate">{fields.account}</p>
          )}
        </div>
        <span className="text-[10px] font-medium bg-accent/10 text-accent px-2 py-0.5 rounded-full">
          TOTP
        </span>
      </div>

      {fields.secret ? (
        <TOTPCode
          secret={fields.secret}
          issuer={fields.issuer}
          algorithm={fields.algorithm}
          digits={parseInt(fields.digits ?? '6', 10)}
          period={parseInt(fields.period ?? '30', 10)}
        />
      ) : (
        <p className="text-xs text-text-tertiary italic">Secret nicht entschlüsselt</p>
      )}
    </div>
  )
}
