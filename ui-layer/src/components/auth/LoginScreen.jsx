import { useState, useMemo } from 'react'
import { useAuth } from '../../context/AuthContext'

/** Convert raw daemon error strings to user-friendly messages. */
function parseAuthError(msg) {
  if (!msg) return null
  const lower = msg.toLowerCase()

  // Try JSON first (daemon sometimes sends structured errors)
  try {
    const j = JSON.parse(msg)
    const text = j.message || j.error || j.msg || ''
    return parseAuthError(text) || { icon: '⚠', title: text || 'Unbekannter Fehler', detail: '' }
  } catch { /* not JSON */ }

  if (lower.includes('invalid password') || lower.includes('authentication failed') || lower.includes('wrong password'))
    return { icon: '🔑', title: 'Falsches Passwort', detail: 'Bitte erneut versuchen.' }
  if (lower.includes('locked') && lower.includes('too many'))
    return { icon: '🔒', title: 'Vault gesperrt', detail: 'Zu viele Fehlversuche. Warte einen Moment.' }
  if (lower.includes('locked'))
    return { icon: '🔒', title: 'Vault ist gesperrt', detail: '' }
  if (lower.includes('timeout') || lower.includes('timed out'))
    return { icon: '⏱', title: 'Zeitüberschreitung', detail: 'Daemon antwortet nicht.' }
  if (lower.includes('not connected') || lower.includes('connection'))
    return { icon: '🔌', title: 'Keine Verbindung', detail: 'Daemon nicht erreichbar.' }

  return { icon: '⚠', title: 'Fehler', detail: msg }
}

export function LoginScreen() {
  const { unlockVault, error, clearError, attemptsRemaining } = useAuth()
  const [password, setPassword] = useState('')
  const [isUnlocking, setIsUnlocking] = useState(false)

  const parsedError = useMemo(() => parseAuthError(error), [error])

  const getLockdownIndicator = () => {
    if (!attemptsRemaining && attemptsRemaining !== 0) return null
    const segments = [1, 2, 3].map(i => (
      <div
        key={i}
        className={`h-4 w-1.5 rounded-sm transition-all duration-300 ${
          i <= attemptsRemaining
            ? i === 1 ? 'bg-danger' : i === 2 ? 'bg-yellow-400' : 'bg-green-400'
            : 'bg-border'
        }`}
      />
    ))
    return (
      <div className="flex items-center gap-1" title={`${attemptsRemaining} Versuche übrig`}>
        {segments}
      </div>
    )
  }

  const handleUnlock = async (e) => {
    e.preventDefault()
    if (!password) return
    clearError()
    setIsUnlocking(true)
    try {
      await unlockVault(password)
    } finally {
      setIsUnlocking(false)
      setPassword('')
    }
  }

  return (
    <div className="min-h-screen bg-cyber-black flex items-center justify-center p-6">
      <div className="max-w-md w-full border border-cyber-border/50 rounded-lg p-8 bg-cyber-dark/80 backdrop-blur-sm shadow-2xl">

        {/* Header */}
        <div className="flex items-center justify-between mb-2">
          <h1 className="font-mono text-2xl font-bold text-cyber-cyan uppercase tracking-wider">
            GRIMLOCKER
          </h1>
          {getLockdownIndicator()}
        </div>
        <p className="font-mono text-xs text-cyber-cyan/60 mb-8">
          Masterpasswort eingeben um den Vault zu entsperren.
        </p>

        <form onSubmit={handleUnlock} className="space-y-5">
          <div>
            <label className="block font-mono text-xs text-cyber-cyan/70 mb-2 uppercase tracking-wide">
              Master Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => { setPassword(e.target.value); if (error) clearError() }}
              placeholder="••••••••••••"
              className="w-full px-4 py-2.5 bg-cyber-dark border border-cyber-border/50 rounded-lg text-cyber-cyan font-mono text-sm placeholder-cyber-cyan/30 focus:outline-none focus:border-cyber-cyan/70 focus:ring-1 focus:ring-cyber-cyan/20 transition"
              disabled={isUnlocking}
              autoFocus
            />
          </div>

          {/* Error display — friendly, not raw JSON */}
          {parsedError && (
            <div className="flex items-start gap-3 p-3 rounded-lg bg-danger/10 border border-danger/30">
              <span className="text-lg leading-none mt-0.5 shrink-0">{parsedError.icon}</span>
              <div className="min-w-0">
                <p className="text-sm font-semibold text-danger">{parsedError.title}</p>
                {parsedError.detail && (
                  <p className="text-xs text-danger/70 mt-0.5">{parsedError.detail}</p>
                )}
              </div>
            </div>
          )}

          <button
            type="submit"
            disabled={isUnlocking || !password}
            className="w-full px-4 py-2.5 bg-cyber-cyan border border-cyber-cyan rounded-lg text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 active:scale-[0.99] transition-all disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isUnlocking ? (
              <span className="flex items-center justify-center gap-2">
                <span className="w-4 h-4 border-2 border-cyber-black/30 border-t-cyber-black rounded-full animate-spin" />
                Entsperren…
              </span>
            ) : 'Vault entsperren'}
          </button>
        </form>

        <p className="font-mono text-xs text-cyber-cyan/30 text-center mt-6">
          Das Passwort wird nur im RAM des Daemons gehalten.
        </p>
      </div>
    </div>
  )
}
