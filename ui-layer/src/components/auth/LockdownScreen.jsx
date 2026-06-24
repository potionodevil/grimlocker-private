import { useState, useEffect, useRef } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'

// SOFORT-HILFE: Grimlocker komplett schliessen und neu starten.
// Der Lockdown-Counter ist in-memory und wird beim Daemon-Neustart zurückgesetzt.

// Formatiert Sekunden als MM:SS oder HH:MM:SS
function formatCountdown(seconds) {
  if (seconds <= 0) return '00:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = seconds % 60
  if (h > 0) return `${String(h).padStart(2,'0')}:${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`
  return `${String(m).padStart(2,'0')}:${String(s).padStart(2,'0')}`
}

function CountdownDisplay({ lockdownUntilMs }) {
  const [remaining, setRemaining] = useState(0)

  useEffect(() => {
    const tick = () => {
      const diff = Math.max(0, Math.floor((lockdownUntilMs - Date.now()) / 1000))
      setRemaining(diff)
    }
    tick()
    const id = setInterval(tick, 1000)
    return () => clearInterval(id)
  }, [lockdownUntilMs])

  const expired = remaining === 0

  return (
    <div className={`rounded-lg border px-6 py-4 text-center ${expired ? 'border-red-500/40 bg-red-500/5' : 'border-yellow-500/30 bg-yellow-500/5'}`}>
      <p className="text-[10px] font-mono uppercase tracking-widest text-text-tertiary mb-1">
        {expired ? 'Sperre abgelaufen — Neustart erlaubt' : 'Sperre läuft ab in'}
      </p>
      <p className={`text-3xl font-mono font-bold tabular-nums tracking-wider ${expired ? 'text-red-400' : 'text-yellow-400'}`}>
        {formatCountdown(remaining)}
      </p>
      {!expired && (
        <p className="text-[10px] text-text-tertiary mt-1">
          Danach können Sie sich mit dem Passwort neu anmelden.
        </p>
      )}
    </div>
  )
}

export function LockdownScreen() {
  const { header, isLockdown, isCritical } = useGrimStore()
  const setLockdownState = useGrimStore((s) => s.setLockdownState)
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const inputRef = useRef(null)

  const lockdownUntilMs = header.lockdownTimestamp > 0
    ? header.lockdownTimestamp * 1000
    : Date.now() + 200 * 60 * 1000

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!password || loading) return
    setLoading(true)
    setError('')

    try {
      await tauriBridge.unlockVault(password)
      // Erfolg: Auth-Flow im tauriBridge/App übernimmt den Rest
    } catch (err) {
      const msg = err?.message ?? ''

      // Lockdown-Metadata aus dem Error in den Store schreiben
      if (err.lockdownUntil != null || err.remaining != null) {
        setLockdownState?.({
          isLockdown: true,
          isCritical: !!err.hardLockdown,
          overrideAttemptsLeft: err.remaining ?? header.overrideAttemptsLeft,
          lockdownTimestamp: err.lockdownUntil ?? header.lockdownTimestamp,
        })
      }

      if (err.hardLockdown || msg.includes('hard lockdown')) {
        setError('Kritischer Lockdown. Grimlocker neu starten.')
      } else if (msg.includes('invalid password')) {
        setError('Falsches Passwort.')
      } else if (msg.includes('too many') || msg.includes('lockdown')) {
        setError('Zu viele Versuche. Warten Sie oder starten Sie neu.')
      } else {
        setError(msg || 'Authentifizierung fehlgeschlagen.')
      }
    } finally {
      setLoading(false)
    }
  }

  // Hard Lockdown — Daemon hat os.Exit aufgerufen, Neustart nötig
  if (isCritical) {
    return (
      <div className="min-h-screen bg-[var(--surface-app)] flex items-center justify-center p-6">
        <div className="w-full max-w-md rounded-2xl border border-red-500/30 bg-red-500/5 p-8 text-center space-y-4">
          <div className="w-12 h-12 rounded-xl bg-red-500/15 flex items-center justify-center mx-auto">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="text-red-400">
              <path d="M12 9v4m0 4h.01M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
            </svg>
          </div>

          <div>
            <h2 className="text-lg font-semibold text-red-400 mb-1">Kritischer Lockdown</h2>
            <p className="text-sm text-text-secondary">
              Alle Override-Versuche aufgebraucht. Der Vault-Dienst wurde gesichert.
            </p>
          </div>

          <div className="rounded-lg border border-border bg-surface-subtle p-4 text-left space-y-2">
            <p className="text-xs font-semibold text-text-primary">So entsperren Sie den Vault wieder:</p>
            <ol className="text-xs text-text-secondary space-y-1 list-decimal list-inside">
              <li>Grimlocker vollständig schliessen</li>
              <li>Grimlocker neu starten</li>
              <li>Normales Passwort eingeben</li>
            </ol>
            <p className="text-[10px] text-text-tertiary mt-2">
              Der Lockdown-Zähler wird beim Neustart zurückgesetzt. Ihre Vault-Daten sind nicht betroffen.
            </p>
          </div>
        </div>
      </div>
    )
  }

  // Soft Lockdown — Countdown + Passwort-Retry
  return (
    <div className="min-h-screen bg-[var(--surface-app)] flex items-center justify-center p-6">
      <div className="w-full max-w-md space-y-5">

        {/* Header */}
        <div className="text-center space-y-1">
          <div className="w-12 h-12 rounded-xl bg-yellow-500/15 flex items-center justify-center mx-auto mb-3">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} className="text-yellow-400">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
            </svg>
          </div>
          <h1 className="text-xl font-bold text-text-primary">Vault gesperrt</h1>
          <p className="text-sm text-text-secondary">
            Zu viele fehlgeschlagene Versuche.
          </p>
        </div>

        {/* Countdown */}
        {isLockdown && <CountdownDisplay lockdownUntilMs={lockdownUntilMs} />}

        {/* Versuche übrig */}
        {header.overrideAttemptsLeft > 0 && (
          <div className="flex items-center justify-between px-4 py-2.5 rounded-lg border border-yellow-500/20 bg-yellow-500/5">
            <span className="text-xs text-text-secondary">Override-Versuche übrig</span>
            <span className="text-sm font-semibold tabular-nums text-yellow-400">
              {header.overrideAttemptsLeft} / 4
            </span>
          </div>
        )}

        {/* Passwort-Form */}
        <form onSubmit={handleSubmit} className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-text-secondary mb-1.5">
              Passwort eingeben
            </label>
            <input
              ref={inputRef}
              type="password"
              value={password}
              onChange={e => { setPassword(e.target.value); setError('') }}
              placeholder="Master-Passwort..."
              className="w-full h-10 px-3 rounded-lg border border-border bg-surface-base text-sm text-text-primary placeholder:text-text-disabled outline-none focus:border-accent transition-colors"
              disabled={loading}
              autoComplete="current-password"
            />
          </div>

          {error && (
            <p className="text-xs text-red-400 px-1">{error}</p>
          )}

          <button
            type="submit"
            disabled={!password || loading}
            className="w-full h-10 rounded-lg bg-accent text-white text-sm font-medium hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? 'Überprüfe...' : 'Entsperren'}
          </button>
        </form>

        {/* Hinweis */}
        <p className="text-[11px] text-text-tertiary text-center px-4">
          Hinweis: Grimlocker neu starten setzt den Lockdown-Zähler zurück,
          falls Sie das Passwort vergessen haben.
        </p>
      </div>
    </div>
  )
}
