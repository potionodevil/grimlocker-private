import { useClipboard } from '../../hooks/useClipboard'

/**
 * CopyButton — Button-Komponente mit eingebautem Clipboard-Schutz und Countdown.
 */
export function CopyButton({ value, label = 'Kopieren', className = '' }) {
  const { copy, countdown, isCopied } = useClipboard()

  const handleClick = async (e) => {
    e.stopPropagation()
    try { await copy(value) } catch { /* ignore */ }
  }

  const base = 'inline-flex items-center gap-1.5 px-2.5 h-7 rounded-md text-xs font-medium transition-fast select-none'
  const style = isCopied
    ? 'bg-green-500/15 text-green-400 border border-green-500/30'
    : 'bg-surface-subtle border border-border text-text-secondary hover:text-text-primary hover:bg-surface-base'

  return (
    <button onClick={handleClick} className={`${base} ${style} ${className}`}>
      {isCopied ? (
        <>
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
            <polyline points="20 6 9 17 4 12" />
          </svg>
          {countdown > 0 ? `Löscht in ${countdown}s` : 'Kopiert!'}
        </>
      ) : (
        <>
          <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <rect x="9" y="9" width="13" height="13" rx="2" /><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
          </svg>
          {label}
        </>
      )}
    </button>
  )
}
