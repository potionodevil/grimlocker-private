import { useState } from 'react'
import { useAuth } from '../../context/AuthContext'

export function LoginScreen() {
  const { unlockVault, error, clearError, attemptsRemaining } = useAuth()
  const [password, setPassword] = useState('')
  const [isUnlocking, setIsUnlocking] = useState(false)

  const getLockdownIndicator = () => {
    const colors = { 3: 'text-cyan-400', 2: 'text-yellow-400', 1: 'text-red-500' }
    const color = colors[attemptsRemaining] || 'text-gray-400'
    const bars = Array(attemptsRemaining || 0).fill('|').join(' ')
    return (
      <span className={`${color} font-mono text-sm`}>
        [ {bars || 'LOCKDOWN'} ]
      </span>
    )
  }

  const handleUnlock = async (e) => {
    e.preventDefault()

    if (!password) {
      return
    }

    clearError()
    setIsUnlocking(true)

    try {
      const result = await unlockVault(password)
      if (!result) {
        // Error handled by context
      }
    } finally {
      setIsUnlocking(false)
      // Clear password from memory immediately
      setPassword('')
    }
  }

  return (
    <div className="min-h-screen bg-cyber-black flex items-center justify-center p-6">
      <div className="max-w-md w-full border border-cyber-border/50 rounded-sm p-8 bg-cyber-dark/80 backdrop-blur-sm">
        <div className="flex items-center justify-between mb-2">
          <h1 className="font-mono text-2xl font-bold text-cyber-cyan uppercase tracking-wider">
            GRIMLOCKER
          </h1>
          {getLockdownIndicator()}
        </div>
        <p className="font-mono text-xs text-cyber-cyan/60 mb-8">
          Enter your master password to unlock the vault.
        </p>

        <form onSubmit={handleUnlock} className="space-y-6">
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
              disabled={isUnlocking}
              autoFocus
            />
          </div>

          {error && (
            <div className="p-3 bg-cyber-red/10 border border-cyber-red/50 rounded">
              <p className="font-mono text-xs text-cyber-red">{error}</p>
            </div>
          )}

          <button
            type="submit"
            disabled={isUnlocking || !password}
            className="w-full px-4 py-2 bg-cyber-cyan border border-cyber-cyan rounded text-cyber-black font-mono text-sm uppercase font-bold hover:bg-cyber-cyan/90 transition disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isUnlocking ? 'Unlocking...' : 'Unlock Vault'}
          </button>
        </form>

        <p className="font-mono text-xs text-cyber-cyan/40 text-center mt-6">
          Your password is sent to the daemon and never stored.
        </p>
      </div>
    </div>
  )
}
