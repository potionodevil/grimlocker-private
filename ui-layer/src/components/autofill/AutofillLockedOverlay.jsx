import { useState, useEffect, useRef } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { ScanLine } from '../shared/ScanLine'

/**
 * AutofillLockedOverlay — Wird angezeigt, wenn der Vault gesperrt ist,
 * der Nutzer aber Strg+G für Autofill gedrückt hat.
 * Fordert zur Eingabe des Master-Passworts auf und entsperrt den Vault.
 */
export function AutofillLockedOverlay({ onUnlock, onCancel }) {
  const [password, setPassword] = useState('')
  const [unlocking, setUnlocking] = useState(false)
  const [error, setError] = useState(null)
  const inputRef = useRef(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const handleSubmit = async (e) => {
    e?.preventDefault()
    if (!password.trim() || unlocking) return
    setUnlocking(true)
    setError(null)
    try {
      await onUnlock(password)
    } catch (err) {
      setError(err?.message || 'Entsperren fehlgeschlagen — Passwort prüfen.')
      setUnlocking(false)
    }
  }

  const handleCancel = () => {
    if (!unlocking) onCancel()
  }

  useEffect(() => {
    const handler = (e) => {
      if (e.key === 'Escape') handleCancel()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  })

  return (
    <AnimatePresence>
      <motion.div
        className="fixed inset-0 z-[999] flex items-center justify-center bg-black/80 backdrop-blur-sm"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        transition={{ duration: 0.15 }}
        onClick={handleCancel}
      >
        <motion.div
          className="relative w-full max-w-sm mx-4 bg-surface-card border border-accent-subtle rounded-xl shadow-2xl overflow-hidden"
          initial={{ scale: 0.95, y: 10 }}
          animate={{ scale: 1, y: 0 }}
          exit={{ scale: 0.95, y: 10 }}
          transition={{ duration: 0.2, ease: 'easeOut' }}
          onClick={(e) => e.stopPropagation()}
        >
          <ScanLine />

          {/* Header */}
          <div className="px-5 pt-5 pb-1">
            <div className="flex items-center gap-3 mb-1">
              <div className="w-8 h-8 rounded-lg bg-accent-subtle flex items-center justify-center">
                <svg className="w-4 h-4 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
                </svg>
              </div>
              <div>
                <p className="text-sm font-semibold text-text-primary">Vault ist gesperrt</p>
                <p className="text-xs text-text-tertiary">Master-Passwort eingeben zum Entsperren</p>
              </div>
            </div>
          </div>

          {/* Form */}
          <form onSubmit={handleSubmit} className="px-5 pb-5 pt-3">
            <input
              ref={inputRef}
              type="password"
              value={password}
              onChange={(e) => { setPassword(e.target.value); setError(null) }}
              placeholder="Master-Passwort..."
              disabled={unlocking}
              autoComplete="off"
              className="w-full px-3 py-2.5 bg-surface-input border border-border-default rounded-lg text-sm text-text-primary placeholder:text-text-tertiary focus:outline-none focus:border-accent transition-colors disabled:opacity-50"
            />

            {error && (
              <motion.p
                className="mt-2 text-xs text-danger"
                initial={{ opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }}
              >
                {error}
              </motion.p>
            )}

            <div className="flex gap-2 mt-3">
              <button
                type="button"
                onClick={handleCancel}
                disabled={unlocking}
                className="flex-1 py-2 px-3 rounded-lg text-xs font-medium text-text-secondary border border-border-default hover:bg-surface-hover transition-colors disabled:opacity-50"
              >
                Abbrechen
              </button>
              <button
                type="submit"
                disabled={!password.trim() || unlocking}
                className="flex-1 py-2 px-3 rounded-lg text-xs font-semibold text-white bg-accent hover:bg-accent-hover transition-colors disabled:opacity-50"
              >
                {unlocking ? 'Entsperren...' : 'Entsperren'}
              </button>
            </div>
          </form>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  )
}
