import { clsx } from 'clsx'
import { StrengthDot } from './StrengthDot'
import { useGrimStore } from '../../store/useGrimStore'

const TYPE_LABELS = {
  password:    'PW',
  ssh:         'SSH',
  cert:        'CERT',
  certificate: 'CERT',
  file_vault:  'FILE',
}

function TypeBadge({ type }) {
  const label = TYPE_LABELS[type] || (type ? type.slice(0, 4).toUpperCase() : '?')
  return (
    <span className="shrink-0 px-1.5 py-0.5 rounded text-xs font-mono font-semibold bg-surface-subtle text-text-tertiary border border-border leading-none">
      {label}
    </span>
  )
}

function relativeTime(nanos) {
  if (!nanos) return '\u2014'
  const ms = nanos / 1e6
  const diff = Date.now() - ms
  const m = Math.floor(diff / 60000)
  if (m < 1)  return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d ago`
  const mo = Math.floor(d / 30)
  return `${mo}mo ago`
}

export function EntryCard({ entry, listView = false, onContextMenu }) {
  const fetchEntry = useGrimStore((s) => s.fetchEntry)

  const handleClick = () => fetchEntry(entry.id)

  const handleContextMenu = (e) => {
    e.preventDefault()
    e.stopPropagation()
    if (onContextMenu) {
      onContextMenu(e, entry)
    }
  }

  if (listView) {
    return (
      <div
        onClick={handleClick}
        onContextMenu={handleContextMenu}
        className="flex items-center gap-4 h-dp-row px-4 border-b border-border cursor-pointer hover:bg-surface-subtle transition-fast group"
      >
        <TypeBadge type={entry.type} />
        <div className="flex-1 min-w-0">
          <p className="text-base font-medium text-text-primary truncate">{entry.title || 'Untitled'}</p>
          <p className="text-sm text-text-secondary truncate">{entry.username || entry.label || '\u2014'}</p>
        </div>
        <StrengthDot score={entry.strength ?? 0} />
        <span className="text-sm text-text-tertiary shrink-0 w-20 text-right">
          {relativeTime(entry.updatedAt)}
        </span>
      </div>
    )
  }

  return (
    <div
      onClick={handleClick}
      onContextMenu={handleContextMenu}
      className={clsx(
        'bg-surface-base border border-border rounded-md shadow-xs p-4',
        'cursor-pointer hover:border-border-strong hover:shadow-sm transition-base',
        'flex flex-col gap-1.5',
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2 min-w-0">
          <TypeBadge type={entry.type} />
          <p className="text-base font-semibold text-text-primary truncate">{entry.title || 'Untitled'}</p>
        </div>
        <button
          onClick={(e) => { e.stopPropagation(); handleContextMenu(e) }}
          className="shrink-0 w-6 h-6 flex items-center justify-center rounded text-text-tertiary hover:bg-surface-subtle hover:text-text-primary transition-fast"
        >
          \u22EF
        </button>
      </div>

      <p className="text-sm text-text-secondary truncate">
        {entry.username || entry.label || '\u2014'}
      </p>

      <div className="flex items-center justify-between pt-0.5">
        <span className="text-sm text-text-tertiary">{relativeTime(entry.updatedAt)}</span>
        <StrengthDot score={entry.strength ?? 0} />
      </div>
    </div>
  )
}