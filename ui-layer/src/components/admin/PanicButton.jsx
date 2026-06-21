import { useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * PanicButton — Admin-Only-Control, das einen sofortigen Hard-Lockdown auslöst.
 *
 * Aktivierung erfordert zwei Bestätigungen:
 *   1. "Bist du sicher?"-Dialog
 *   2. Passphrase-Eingabe zur Admin-Identitätsprüfung
 *
 * Nach der Aktivierung zerstört der Server sämtliches Key-Material und die App lädt neu.
 * Nur rendern, wenn der aktuelle User die "admin"-Rolle hat.
 */
export function PanicButton({ userRoles = [] }) {
  const isAdmin = userRoles.includes('admin')

  const [step, setStep] = useState('idle') // idle | confirm | passphrase | activating | done
  const [passphrase, setPassphrase] = useState('')
  const [error, setError] = useState('')

  if (!isAdmin) return null

  const handleFirstConfirm = () => setStep('passphrase')
  const handleCancel = () => {
    setStep('idle')
    setPassphrase('')
    setError('')
  }

  const handleActivate = async () => {
    if (!passphrase.trim()) {
      setError('Passphrase darf nicht leer sein.')
      return
    }
    setStep('activating')
    setError('')

    try {
      await tauriBridge.activatePanicButton(passphrase)
      setStep('done')
      // Dem Server einen Moment geben, den Lockdown abzuschliessen, dann neuladen
      setTimeout(() => window.location.reload(), 2000)
    } catch (e) {
      setError(e.message || 'Aktivierung fehlgeschlagen.')
      setStep('passphrase')
    }
  }

  return (
    <>
      {/* Panic trigger button */}
      <button
        onClick={() => setStep('confirm')}
        className="inline-flex items-center gap-2 px-4 py-2 rounded-lg
                   bg-red-900/40 hover:bg-red-900/60
                   border border-red-700/50 hover:border-red-600
                   text-red-400 hover:text-red-300
                   text-sm font-semibold transition-colors"
      >
        <span className="font-mono text-base leading-none">[!]</span>
        PANIC — Vault sichern
      </button>

      <AnimatePresence>
        {step !== 'idle' && (
          <motion.div
            key="overlay"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-[110] flex items-center justify-center"
            style={{ background: 'rgba(0,0,0,0.75)' }}
          >
            <motion.div
              initial={{ scale: 0.9, opacity: 0 }}
              animate={{ scale: 1, opacity: 1 }}
              exit={{ scale: 0.9, opacity: 0 }}
              transition={{ duration: 0.18 }}
              className="bg-surface-base border border-red-700/40 rounded-xl shadow-2xl
                         w-full max-w-md mx-4 overflow-hidden"
            >
              {/* Red top strip */}
              <div className="h-1.5 w-full bg-red-600" />

              <div className="p-6">
                {/* Confirm step */}
                {step === 'confirm' && (
                  <>
                    <h2 className="text-base font-bold text-red-400 mb-2">
                      Vault sichern — Letzte Warnung
                    </h2>
                    <p className="text-sm text-text-secondary mb-1">
                      Der PANIC-Button loescht sofort alle Schluessel aus dem Speicher
                      und ueberschreibt die Datenbank mit Rauschen.
                    </p>
                    <p className="text-sm text-text-secondary mb-6">
                      Dieser Vorgang ist nicht rueckgaengig zu machen.
                      Alle Daten sind dauerhaft nicht mehr zugreifbar.
                    </p>
                    <div className="flex justify-end gap-3">
                      <button
                        onClick={handleCancel}
                        className="px-4 py-2 text-sm font-medium rounded-md
                                   bg-surface-subtle border border-border text-text-secondary
                                   hover:text-text-primary transition-colors"
                      >
                        Abbrechen
                      </button>
                      <button
                        onClick={handleFirstConfirm}
                        className="px-4 py-2 text-sm font-bold rounded-md
                                   bg-red-700 hover:bg-red-600 text-white transition-colors"
                      >
                        Weiter
                      </button>
                    </div>
                  </>
                )}

                {/* Passphrase step */}
                {(step === 'passphrase' || step === 'activating') && (
                  <>
                    <h2 className="text-base font-bold text-red-400 mb-2">
                      Passphrase zur Bestaetigung
                    </h2>
                    <p className="text-sm text-text-secondary mb-4">
                      Gib deine Admin-Passphrase ein um den Vorgang zu bestaetigen.
                    </p>
                    <input
                      type="password"
                      value={passphrase}
                      onChange={(e) => setPassphrase(e.target.value)}
                      onKeyDown={(e) => e.key === 'Enter' && handleActivate()}
                      placeholder="Passphrase eingeben"
                      disabled={step === 'activating'}
                      autoFocus
                      className="w-full h-9 px-3 mb-1 rounded-md text-sm
                                 bg-surface-app border border-border
                                 text-text-primary placeholder:text-text-disabled
                                 focus:outline-none focus:ring-2 focus:ring-red-500/30 focus:border-red-500/50
                                 disabled:opacity-50"
                    />
                    {error && (
                      <p className="text-xs text-red-400 mb-3">{error}</p>
                    )}
                    <div className="flex justify-end gap-3 mt-4">
                      <button
                        onClick={handleCancel}
                        disabled={step === 'activating'}
                        className="px-4 py-2 text-sm font-medium rounded-md
                                   bg-surface-subtle border border-border text-text-secondary
                                   hover:text-text-primary transition-colors disabled:opacity-50"
                      >
                        Abbrechen
                      </button>
                      <button
                        onClick={handleActivate}
                        disabled={step === 'activating' || !passphrase.trim()}
                        className="px-4 py-2 text-sm font-bold rounded-md
                                   bg-red-700 hover:bg-red-600 text-white transition-colors
                                   disabled:opacity-50 disabled:cursor-not-allowed"
                      >
                        {step === 'activating' ? 'Wird ausgefuehrt...' : 'Vault sichern'}
                      </button>
                    </div>
                  </>
                )}

                {/* Done step */}
                {step === 'done' && (
                  <div className="flex flex-col items-center py-4 text-center">
                    <p className="text-base font-bold text-red-400 mb-2">
                      Vault wurde gesichert
                    </p>
                    <p className="text-sm text-text-secondary">
                      Alle Schluessel wurden geloescht. Die App wird neu gestartet...
                    </p>
                  </div>
                )}
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>
    </>
  )
}
