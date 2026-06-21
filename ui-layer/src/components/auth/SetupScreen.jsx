import { useState } from 'react'
import { useAuth } from '../../context/AuthContext'

export function SetupScreen() {
  const { initializeVault, confirmVaultReady, error, clearError } = useAuth()
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [recoveryPhrase, setRecoveryPhrase] = useState(null)
  const [isInitializing, setIsInitializing] = useState(false)
  const [copied, setCopied] = useState(false)

  const handleInitialize = async (e) => {
    e.preventDefault()

    if (!password || !confirmPassword) {
      return
    }

    if (password !== confirmPassword) {
      return
    }

    if (password.length < 12) {
      return
    }

    clearError()
    setIsInitializing(true)

    try {
      // Backend aufrufen, um den Vault zu initialisieren
      // Das gibt die Recovery-Phrase zurück — die zeigen wir dem User genau EINMAL
      const phrase = await initializeVault(password)
      if (phrase) {
        setRecoveryPhrase(phrase)
      }
    } finally {
      setIsInitializing(false)
      // Passwort aus dem RAM entfernen — sicherheitshalber
      setPassword('')
      setConfirmPassword('')
    }
  }

  const handleCopyPhrase = () => {
    if (recoveryPhrase) {
      navigator.clipboard.writeText(recoveryPhrase)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    }
  }

  if (recoveryPhrase) {
    return (
      <div className="min-h-screen bg-cyber-black flex items-center justify-center p-6">
        <div className="max-w-2xl w-full border border-cyber-border/50 rounded-sm p-8 bg-cyber-dark/80 backdrop-blur-sm">
          <h1 className="font-mono text-2xl font-bold text-cyber-cyan mb-6 uppercase tracking-wider">
            Recovery Phrase
          </h1>

          <div className="bg-cyber-dark border border-cyber-amber/30 rounded-sm p-6 mb-8">
            <p className="font-mono text-xs text-cyber-amber/70 mb-4 uppercase">
              ⚠️ Save this phrase in a secure location. It cannot be recovered.
            </p>
            <div className="font-mono text-sm text-cyber-cyan bg-cyber-black/50 p-4 rounded border border-cyber-border/30 mb-4 break-words">
              {recoveryPhrase}
            </div>
            <button
              onClick={handleCopyPhrase}
              className="w-full px-4 py-2 bg-cyber-cyan/20 border border-cyber-cyan/50 rounded text-cyber-cyan font-mono text-xs uppercase hover:bg-cyber-cyan/30 transition"
            >
              {copied ? '✓ Copied' : 'Copy to Clipboard'}
            </button>
          </div>

          <p className="font-mono text-xs text-cyber-cyan/60 text-center mb-6">
            You will be redirected to the vault after confirming.
          </p>

          <button
            onClick={() => {
              confirmVaultReady()
            }}
            className="w-full px-4 py-2 bg-cyber-cyan border border-cyber-cyan rounded text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 transition"
          >
            I have saved the recovery phrase
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-cyber-black flex items-center justify-center p-6">
      <div className="max-w-md w-full border border-cyber-border/50 rounded-sm p-8 bg-cyber-dark/80 backdrop-blur-sm">
        <h1 className="font-mono text-2xl font-bold text-cyber-cyan mb-2 uppercase tracking-wider">
          Initialize Vault
        </h1>
        <p className="font-mono text-xs text-cyber-cyan/60 mb-8">
          Create your master password to begin.
        </p>

        <form onSubmit={handleInitialize} className="space-y-6">
          <div>
            <label className="block font-mono text-xs text-cyber-cyan/70 mb-2 uppercase">
              Master Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••••••"
              className="w-full px-4 py-2 bg-cyber-dark border border-cyber-border/50 rounded text-cyber-cyan font-mono text-sm placeholder-cyber-cyan/30 focus:outline-none focus:border-cyber-cyan/70 transition"
              disabled={isInitializing}
            />
            <p className="font-mono text-xs text-cyber-cyan/40 mt-2">
              Minimum 12 characters
            </p>
          </div>

          <div>
            <label className="block font-mono text-xs text-cyber-cyan/70 mb-2 uppercase">
              Confirm Password
            </label>
            <input
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="••••••••••••"
              className="w-full px-4 py-2 bg-cyber-dark border border-cyber-border/50 rounded text-cyber-cyan font-mono text-sm placeholder-cyber-cyan/30 focus:outline-none focus:border-cyber-cyan/70 transition"
              disabled={isInitializing}
            />
          </div>

          {error && (
            <div className="p-3 bg-cyber-red/10 border border-cyber-red/50 rounded">
              <p className="font-mono text-xs text-cyber-red">{error}</p>
            </div>
          )}

          <button
            type="submit"
            disabled={
              isInitializing ||
              !password ||
              !confirmPassword ||
              password !== confirmPassword ||
              password.length < 12
            }
            className="w-full px-4 py-2 bg-cyber-cyan border border-cyber-cyan rounded text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 transition disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isInitializing ? 'Initializing...' : 'Initialize Vault'}
          </button>
        </form>
      </div>
    </div>
  )
}
