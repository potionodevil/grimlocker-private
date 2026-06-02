import { useEffect, useRef, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'

// Parse raw daemon/system error strings into a user-friendly message
function parseError(raw) {
  if (!raw) return null
  let msg = raw

  // Strip JSON wrappers
  try {
    const j = JSON.parse(raw)
    msg = j.message || j.error || j.msg || j.detail || raw
  } catch { /* not JSON */ }

  const lower = msg.toLowerCase()
  const isPanic = lower.includes('panic') || lower.includes('critical') || lower.includes('zeroize')

  // Map known patterns to clean messages
  let friendly = msg
  if (lower.includes('connection refused') || lower.includes('not connected'))
    friendly = 'Daemon not reachable — is Grimlocker running?'
  else if (lower.includes('timeout') || lower.includes('timed out'))
    friendly = 'Connection timed out. Retrying…'
  else if (lower.includes('authentication failed') || lower.includes('invalid password'))
    friendly = 'Authentication failed'
  else if (lower.includes('permission denied') || lower.includes('access denied'))
    friendly = 'Permission denied'
  else if (lower.includes('disk') || lower.includes('storage') || lower.includes('no space'))
    friendly = 'Storage error — check available disk space'
  else if (lower.includes('zeroize') || lower.includes('panic'))
    friendly = 'PANIC — vault has been zeroized'
  else if (msg.length > 120)
    // Truncate overly long raw errors
    friendly = msg.slice(0, 100) + '…'

  return { text: friendly, isPanic }
}

export function TerminalError({ message, onDismiss }) {
  const parsed = parseError(message)
  const [visible, setVisible] = useState(false)
  const timerRef = useRef(null)

  useEffect(() => {
    if (!parsed) { setVisible(false); return }
    setVisible(true)
    clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => {
      setVisible(false)
      onDismiss?.()
    }, 5500)
    return () => clearTimeout(timerRef.current)
  }, [message]) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <AnimatePresence>
      {visible && parsed && (
        <motion.div
          key={message}
          initial={{ opacity: 0, y: -16, scale: 0.97 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          exit={{ opacity: 0, y: -12, scale: 0.97 }}
          transition={{ type: 'spring', stiffness: 400, damping: 28 }}
          className="fixed top-4 left-1/2 -translate-x-1/2 z-[200] max-w-md w-full mx-4"
          style={{ pointerEvents: 'auto' }}
        >
          <div
            className={[
              'flex items-start gap-3 px-4 py-3 rounded-xl border shadow-lg backdrop-blur-md',
              parsed.isPanic
                ? 'bg-red-500/10 border-red-500/40 text-red-400'
                : 'bg-amber-500/10 border-amber-500/40 text-amber-400',
            ].join(' ')}
          >
            {/* Icon */}
            <div className="mt-0.5 shrink-0">
              {parsed.isPanic ? (
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                  <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0zM12 9v4M12 17h.01" />
                </svg>
              ) : (
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
                  <circle cx="12" cy="12" r="10" />
                  <line x1="12" y1="8" x2="12" y2="12" />
                  <line x1="12" y1="16" x2="12.01" y2="16" />
                </svg>
              )}
            </div>

            {/* Message */}
            <div className="flex-1 min-w-0">
              <p className={`text-xs font-semibold uppercase tracking-wide mb-0.5 ${parsed.isPanic ? 'text-red-300' : 'text-amber-300'}`}>
                {parsed.isPanic ? 'Critical' : 'Error'}
              </p>
              <p className="text-sm text-text-primary leading-snug">{parsed.text}</p>
            </div>

            {/* Dismiss */}
            <button
              onClick={() => { setVisible(false); onDismiss?.() }}
              className="mt-0.5 shrink-0 p-0.5 rounded hover:opacity-70 transition-opacity"
              aria-label="Dismiss"
            >
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round">
                <path d="M18 6L6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          {/* Auto-dismiss progress bar */}
          <motion.div
            initial={{ scaleX: 1 }}
            animate={{ scaleX: 0 }}
            transition={{ duration: 5.5, ease: 'linear' }}
            className={`h-0.5 mt-0.5 rounded-full origin-left ${parsed.isPanic ? 'bg-red-500/50' : 'bg-amber-500/50'}`}
          />
        </motion.div>
      )}
    </AnimatePresence>
  )
}
