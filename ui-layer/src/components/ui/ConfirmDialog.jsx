import { useEffect, useRef } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { Button } from './Button'

/**
 * ConfirmDialog — Unser eigener, gestylter Confirm-Dialog (ersetzt window.confirm()).
 * Warum? Native confirm()-Dialoge lassen sich nicht stylen und sind immer Englisch.
 *
 * Props:
 *   isOpen        boolean   — Sichtbarkeit
 *   title         string    — Dialog-Überschrift
 *   message       string    — Body-Text (gerne auch auf Deutsch)
 *   confirmLabel  string    — Confirm-Button-Label (default: "Confirm")
 *   cancelLabel   string    — Cancel-Button-Label (default: "Cancel")
 *   variant       string    — "danger" (roter Confirm) | "primary" (default)
 *   onConfirm     function  — wird bei Bestätigung aufgerufen
 *   onCancel      function  — wird bei Abbruch oder ESC aufgerufen
 */
export function ConfirmDialog({
  isOpen,
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  variant = 'danger',
  onConfirm,
  onCancel,
}) {
  const cancelRef = useRef(null)

  // Fokus auf Cancel-Button beim Öffnen — sicherer Default bei destruktiven Aktionen
  useEffect(() => {
    if (isOpen && cancelRef.current) {
      cancelRef.current.focus()
    }
  }, [isOpen])

  // Tastatur: ESC = abbrechen, Enter = bestätigen (nur wenn Confirm fokussiert ist)
  useEffect(() => {
    if (!isOpen) return
    const handleKey = (e) => {
      if (e.key === 'Escape') onCancel?.()
      if (e.key === 'Enter' && document.activeElement?.dataset?.confirm === 'true') {
        onConfirm?.()
      }
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [isOpen, onConfirm, onCancel])

  return (
    <AnimatePresence>
      {isOpen && (
        // Backdrop
        <motion.div
          key="backdrop"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.15 }}
          className="fixed inset-0 z-[100] flex items-center justify-center"
          style={{ background: 'rgba(0,0,0,0.55)' }}
          onClick={onCancel}
        >
          {/* Dialog panel */}
          <motion.div
            key="dialog"
            initial={{ opacity: 0, scale: 0.94, y: 8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.94, y: 8 }}
            transition={{ duration: 0.18, ease: 'easeOut' }}
            className="bg-surface-base rounded-xl shadow-2xl border border-border w-full max-w-sm mx-4 overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header strip */}
            {variant === 'danger' && (
              <div className="h-1 w-full bg-red-500/70" />
            )}

            <div className="p-6">
              {/* Title */}
              <h2 className={[
                'text-base font-semibold mb-2',
                variant === 'danger' ? 'text-red-400' : 'text-text-primary',
              ].join(' ')}>
                {title}
              </h2>

              {/* Message */}
              <p className="text-sm text-text-secondary leading-relaxed mb-6">
                {message}
              </p>

              {/* Actions */}
              <div className="flex justify-end gap-3">
                <button
                  ref={cancelRef}
                  onClick={onCancel}
                  className="px-4 py-2 text-sm font-medium text-text-secondary
                             bg-surface-subtle border border-border rounded-md
                             hover:bg-surface-base hover:text-text-primary
                             transition-colors focus:outline-none focus:ring-2
                             focus:ring-accent/40"
                >
                  {cancelLabel}
                </button>
                <button
                  data-confirm="true"
                  onClick={onConfirm}
                  className={[
                    'px-4 py-2 text-sm font-medium rounded-md transition-colors',
                    'focus:outline-none focus:ring-2',
                    variant === 'danger'
                      ? 'bg-red-600 hover:bg-red-700 text-white focus:ring-red-500/40'
                      : 'bg-accent hover:bg-accent-hover text-text-inverse focus:ring-accent/40',
                  ].join(' ')}
                >
                  {confirmLabel}
                </button>
              </div>
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
