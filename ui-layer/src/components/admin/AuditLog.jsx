import { useGrimStore } from '../../store/useGrimStore'
import { Badge } from '../ui/Badge'

const ACTION_VARIANT = {
  'vault.unlock':    'accent',
  'storage.write':   'neutral',
  'storage.read':    'neutral',
  'storage.delete':  'danger',
  'integrity.check': 'success',
}

function formatTime(ms) {
  return new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function AuditLog() {
  const { operationsLog } = useGrimStore()

  const rows = operationsLog.length > 0
    ? operationsLog.slice(0, 50).map((op, i) => ({
        id:        i,
        user:      op.user || 'system',
        action:    op.type,
        resource:  op.detail || '—',
        timestamp: formatTime(op.time),
      }))
    : DEMO_ROWS

  return (
    <div>
      <h2 className="text-lg font-semibold text-text-primary mb-3">Audit Log</h2>
      <div className="bg-surface-base border border-border rounded-md overflow-hidden shadow-xs">
        {/* Header */}
        <div className="grid grid-cols-[1fr_1.5fr_1.5fr_auto] gap-4 px-4 h-9 items-center border-b border-border">
          {['User', 'Action', 'Resource', 'Timestamp'].map((h) => (
            <span key={h} className="text-sm font-medium text-text-tertiary">{h}</span>
          ))}
        </div>

        {/* Rows */}
        {rows.map((row) => (
          <div
            key={row.id}
            className="grid grid-cols-[1fr_1.5fr_1.5fr_auto] gap-4 px-4 h-dp-row items-center border-b border-border last:border-0 hover:bg-surface-subtle transition-fast"
          >
            <span className="text-sm text-text-primary font-medium truncate">{row.user}</span>
            <Badge variant={ACTION_VARIANT[row.action] ?? 'neutral'}>{row.action}</Badge>
            <span className="text-sm text-text-secondary truncate">{row.resource}</span>
            <span className="text-sm text-text-tertiary tabular-nums">{row.timestamp}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

const DEMO_ROWS = [
  { id: 0, user: 'admin',  action: 'vault.unlock',    resource: '/vault',       timestamp: '10:41:02' },
  { id: 1, user: 'john',   action: 'storage.read',    resource: 'entry#4a2f',   timestamp: '10:40:58' },
  { id: 2, user: 'admin',  action: 'storage.write',   resource: 'entry#9b1c',   timestamp: '10:38:14' },
  { id: 3, user: 'system', action: 'integrity.check', resource: 'daemon',       timestamp: '10:35:00' },
  { id: 4, user: 'john',   action: 'storage.delete',  resource: 'entry#3e7d',   timestamp: '10:30:11' },
]
