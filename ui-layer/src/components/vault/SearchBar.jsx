import { AnimatePresence, motion } from 'framer-motion'
import { useEffect, useRef, useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'

export function SearchBar({ open, onClose }) {
  const [query, setQuery] = useState('')
  const { entries, setActiveEntry } = useGrimStore()
  const inputRef = useRef(null)

  useEffect(() => {
    if (open) {
      setQuery('')
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose?.() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  const results = query.trim()
    ? entries.filter((e) =>
        [e.title, e.username, e.label, e.url]
          .filter(Boolean)
          .some((v) => v.toLowerCase().includes(query.toLowerCase()))
      ).slice(0, 8)
    : entries.slice(0, 6)

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            key="sb-bg"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15 }}
            className="fixed inset-0 bg-black/30 z-40"
            onClick={onClose}
          />

          <motion.div
            key="sb-panel"
            initial={{ opacity: 0, scale: 0.97, y: -8 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.97, y: -8 }}
            transition={{ duration: 0.15, ease: [0.16, 1, 0.3, 1] }}
            className="fixed top-24 left-1/2 -translate-x-1/2 w-full max-w-xl bg-surface-overlay border border-border rounded-lg shadow-md z-50 overflow-hidden"
          >
            {/* Input */}
            <div className="flex items-center gap-3 px-4 h-12 border-b border-border">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--text-tertiary)" strokeWidth="2">
                <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
              </svg>
              <input
                ref={inputRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search everything…"
                className="flex-1 bg-transparent text-base text-text-primary placeholder:text-text-tertiary outline-none"
              />
              {query && (
                <button onClick={() => setQuery('')} className="text-text-tertiary hover:text-text-primary">✕</button>
              )}
            </div>

            {/* Results */}
            <div className="max-h-80 overflow-y-auto py-1">
              {results.length === 0 ? (
                <p className="px-4 py-6 text-center text-sm text-text-tertiary">No results</p>
              ) : (
                results.map((entry) => (
                  <button
                    key={entry.id}
                    onClick={() => { setActiveEntry(entry); onClose() }}
                    className="w-full flex items-center gap-3 px-4 h-11 text-left hover:bg-surface-subtle transition-fast"
                  >
                    <span className="text-base">🔑</span>
                    <div className="flex-1 min-w-0">
                      <p className="text-sm font-medium text-text-primary truncate">{entry.title || 'Untitled'}</p>
                      <p className="text-sm text-text-tertiary truncate">{entry.username || '—'}</p>
                    </div>
                  </button>
                ))
              )}
            </div>

            <div className="px-4 py-2 border-t border-border flex items-center gap-4 text-sm text-text-tertiary">
              <span><kbd className="font-sans">↑↓</kbd> navigate</span>
              <span><kbd className="font-sans">↵</kbd> open</span>
              <span><kbd className="font-sans">Esc</kbd> close</span>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}
