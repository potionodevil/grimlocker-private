import { useState, useMemo } from 'react'
import { useAuth } from '../../context/AuthContext'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * Wandelt rohe Daemon-Fehlerstrings in benutzerfreundliche Nachrichten um.
 * Der Daemon schickt manchmal JSON, manchmal Plaintext — das hier normalisiert beides.
 * Die deutschen Texte sind bewusst gewählt, weil die UI auf Deutsch ist.
 */
function parseAuthError(msg) {
  if (!msg) return null
  const lower = msg.toLowerCase()

  // Erst versuchen, JSON zu parsen — der Daemon schickt manchmal strukturierte Fehler
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

// ── Recovery Modal ──────────────────────────────────────────────────────────

function RecoveryModal({ onClose }) {
  const [step, setStep] = useState('phrase') // 'phrase' | 'newpass' | 'done'
  const [phrase, setPhrase] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [newRecoveryPhrase, setNewRecoveryPhrase] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleVerifyPhrase = (e) => {
    e.preventDefault()
    if (phrase.trim().length < 50) {
      setError('Recovery-Phrase ist zu kurz.')
      return
    }
    setError('')
    setStep('newpass')
  }

  const handleReset = async (e) => {
    e.preventDefault()
    if (newPassword !== confirmPassword) { setError('Passwörter stimmen nicht überein.'); return }
    if (newPassword.length < 8) { setError('Passwort muss mindestens 8 Zeichen haben.'); return }
    setError('')
    setLoading(true)
    try {
      const result = await tauriBridge.changePasswordWithRecovery(phrase.trim(), newPassword)
      setNewRecoveryPhrase(result.new_recovery_phrase)
      setStep('done')
    } catch (err) {
      const msg = err?.message ?? ''
      if (msg.includes('invalid recovery phrase')) {
        setError('Recovery-Phrase ungültig. Bitte zurück und erneut prüfen.')
        setStep('phrase')
      } else if (msg.includes('not initialized')) {
        setError('Vault nicht initialisiert. Bitte Grimlocker neu starten.')
      } else {
        setError(msg || 'Fehler beim Zurücksetzen.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-6 bg-black/80 backdrop-blur-sm">
      <div className="w-full max-w-lg bg-cyber-dark border border-cyber-border/60 rounded-xl shadow-2xl overflow-hidden">

        {/* Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-cyber-border/40">
          <h2 className="font-mono text-sm font-bold text-cyber-cyan uppercase tracking-wider">
            {step === 'done' ? 'Passwort zurückgesetzt' : 'Passwort vergessen'}
          </h2>
          <button onClick={onClose} className="text-cyber-cyan/40 hover:text-cyber-cyan/80 transition-colors text-lg leading-none">&times;</button>
        </div>

        <div className="p-6 space-y-5">

          {/* Step 1: Recovery phrase input */}
          {step === 'phrase' && (
            <form onSubmit={handleVerifyPhrase} className="space-y-4">
              <div className="p-3 rounded-lg bg-yellow-500/10 border border-yellow-500/30">
                <p className="font-mono text-xs text-yellow-400/90 leading-relaxed">
                  ⚠ Alle verschlüsselten Einträge werden beim Zurücksetzen unwiderruflich gelöscht.
                  Stellen Sie sicher, dass Sie kein Backup benötigen.
                </p>
              </div>
              <div>
                <label className="block font-mono text-xs text-cyber-cyan/70 mb-2 uppercase tracking-wide">
                  Recovery-Phrase (200 Zeichen aus dem Setup)
                </label>
                <textarea
                  value={phrase}
                  onChange={e => { setPhrase(e.target.value); setError('') }}
                  rows={4}
                  placeholder="Ihre Recovery-Phrase eingeben..."
                  className="w-full px-4 py-2.5 bg-cyber-dark border border-cyber-border/50 rounded-lg text-cyber-cyan font-mono text-xs placeholder-cyber-cyan/30 focus:outline-none focus:border-cyber-cyan/70 focus:ring-1 focus:ring-cyber-cyan/20 transition resize-none"
                />
              </div>
              {error && <p className="font-mono text-xs text-red-400">{error}</p>}
              <button
                type="submit"
                disabled={!phrase.trim()}
                className="w-full px-4 py-2.5 bg-cyber-cyan border border-cyber-cyan rounded-lg text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 transition disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Weiter
              </button>
            </form>
          )}

          {/* Step 2: New password */}
          {step === 'newpass' && (
            <form onSubmit={handleReset} className="space-y-4">
              <p className="font-mono text-xs text-cyber-cyan/60">
                Recovery-Phrase akzeptiert. Neues Master-Passwort festlegen:
              </p>
              <div>
                <label className="block font-mono text-xs text-cyber-cyan/70 mb-2 uppercase tracking-wide">Neues Passwort</label>
                <input
                  type="password"
                  value={newPassword}
                  onChange={e => { setNewPassword(e.target.value); setError('') }}
                  placeholder="Mindestens 8 Zeichen..."
                  className="w-full px-4 py-2.5 bg-cyber-dark border border-cyber-border/50 rounded-lg text-cyber-cyan font-mono text-sm placeholder-cyber-cyan/30 focus:outline-none focus:border-cyber-cyan/70 focus:ring-1 focus:ring-cyber-cyan/20 transition"
                  autoFocus
                />
              </div>
              <div>
                <label className="block font-mono text-xs text-cyber-cyan/70 mb-2 uppercase tracking-wide">Passwort bestätigen</label>
                <input
                  type="password"
                  value={confirmPassword}
                  onChange={e => { setConfirmPassword(e.target.value); setError('') }}
                  placeholder="Passwort wiederholen..."
                  className="w-full px-4 py-2.5 bg-cyber-dark border border-cyber-border/50 rounded-lg text-cyber-cyan font-mono text-sm placeholder-cyber-cyan/30 focus:outline-none focus:border-cyber-cyan/70 focus:ring-1 focus:ring-cyber-cyan/20 transition"
                />
              </div>
              {error && <p className="font-mono text-xs text-red-400">{error}</p>}
              <div className="flex gap-3">
                <button
                  type="button"
                  onClick={() => setStep('phrase')}
                  className="flex-1 px-4 py-2.5 border border-cyber-border/50 rounded-lg text-cyber-cyan/70 font-mono text-sm uppercase hover:border-cyber-cyan/50 transition"
                >
                  Zurück
                </button>
                <button
                  type="submit"
                  disabled={!newPassword || !confirmPassword || loading}
                  className="flex-1 px-4 py-2.5 bg-cyber-cyan border border-cyber-cyan rounded-lg text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 transition disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  {loading ? 'Wird zurückgesetzt…' : 'Passwort zurücksetzen'}
                </button>
              </div>
            </form>
          )}

          {/* Step 3: Done — show new recovery phrase */}
          {step === 'done' && (
            <div className="space-y-4">
              <div className="p-3 rounded-lg bg-green-500/10 border border-green-500/30">
                <p className="font-mono text-xs text-green-400 font-bold mb-1">✓ Passwort erfolgreich zurückgesetzt</p>
                <p className="font-mono text-xs text-green-400/70">
                  Sie können sich jetzt mit dem neuen Passwort einloggen.
                </p>
              </div>
              <div>
                <p className="font-mono text-xs text-cyber-cyan/70 uppercase tracking-wide mb-2 font-bold">
                  Neue Recovery-Phrase — jetzt sicher aufbewahren:
                </p>
                <div className="p-3 rounded-lg bg-black/40 border border-cyber-border/40">
                  <p className="font-mono text-xs text-cyber-cyan/90 break-all leading-relaxed select-all">
                    {newRecoveryPhrase}
                  </p>
                </div>
                <p className="font-mono text-[10px] text-red-400/70 mt-2">
                  ⚠ Diese Phrase wird nur einmal angezeigt. Kopieren Sie sie jetzt.
                </p>
              </div>
              <button
                onClick={onClose}
                className="w-full px-4 py-2.5 bg-cyber-cyan border border-cyber-cyan rounded-lg text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 transition"
              >
                Zum Login
              </button>
            </div>
          )}

        </div>
      </div>
    </div>
  )
}

// ──────────────────────────────────────────────────────────────────────────────

export function LoginScreen() {
  const { unlockVault, error, clearError, attemptsRemaining } = useAuth()
  const [password, setPassword] = useState('')
  const [isUnlocking, setIsUnlocking] = useState(false)
  const [showRecovery, setShowRecovery] = useState(false)

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
    <>
    {showRecovery && <RecoveryModal onClose={() => setShowRecovery(false)} />}
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

          {/* Fehleranzeige — immer benutzerfreundlich, nie rohes JSON */}
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

        <div className="flex items-center justify-between mt-6">
          <p className="font-mono text-xs text-cyber-cyan/30">
            Passwort nur im RAM.
          </p>
          <button
            type="button"
            onClick={() => setShowRecovery(true)}
            className="font-mono text-xs text-cyber-cyan/40 hover:text-cyber-cyan/70 underline underline-offset-2 transition-colors"
          >
            Passwort vergessen?
          </button>
        </div>
      </div>
    </div>
    </>
  )
}
