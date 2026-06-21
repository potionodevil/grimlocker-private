import { useState, useEffect } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import { Badge } from '../ui/Badge'

// SecurityEvent-Level → Badge-Variante mappen
const LEVEL_VARIANT = {
  INFO:     'neutral',
  WARN:     'warning',
  CRITICAL: 'danger',
}

// Map Modul-/Message-Patterns auf Action-Labels — so wissen wir, was passiert ist
function inferAction(event) {
  const msg = (event.message || '').toLowerCase()
  const mod = (event.module || '').toLowerCase()
  if (msg.includes('unlock') || msg.includes('auth.unlock')) return 'vault.unlock'
  if (msg.includes('sync')) return 'sync'
  if (msg.includes('write') || msg.includes('save')) return 'storage.write'
  if (msg.includes('read') || msg.includes('list')) return 'storage.read'
  if (msg.includes('delete') || msg.includes('wipe')) return 'storage.delete'
  if (msg.includes('integrity') || mod.includes('integrity')) return 'integrity.check'
  if (msg.includes('logout') || msg.includes('lock')) return 'vault.lock'
  return mod || 'system'
}

const ACTION_VARIANT = {
  'vault.unlock':    'accent',
  'vault.lock':      'neutral',
  'storage.write':   'neutral',
  'storage.read':    'neutral',
  'storage.delete':  'danger',
  'integrity.check': 'success',
  'sync':            'accent',
}

function formatTime(ns) {
  // SecurityEvent-Timestamps sind UnixNano — müssen wir erst umrechnen
  const ms = ns > 1e15 ? Math.floor(ns / 1e6) : ns
  return new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export function AuditLog() {
  const daemonStatus = useGrimStore((s) => s.daemonStatus)
  const [events, setEvents]   = useState(null)   // null = loading, [] = empty/error
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    if (daemonStatus !== 'online') return
    setLoading(true)
    tauriBridge.listAuditEntries(50)
      .then(data => setEvents(Array.isArray(data) ? data : []))
      .catch(() => setEvents(null))   // Bei Fehler Demo-Daten anzeigen (produktiv nicht, aber für Dev okay)
      .finally(() => setLoading(false))
  }, [daemonStatus])

  const rows = (events && events.length > 0)
    ? events.slice().reverse().map((ev, i) => ({
        id:        i,
        user:      ev.subject_id || ev.module || 'system',
        action:    inferAction(ev),
        resource:  ev.message || '—',
        timestamp: formatTime(ev.timestamp),
        level:     ev.level || 'INFO',
      }))
    : DEMO_ROWS

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-text-primary">Audit Log</h2>
        {loading && (
          <span className="text-xs text-text-tertiary flex items-center gap-1.5">
            <span className="w-3 h-3 border-2 border-accent border-t-transparent rounded-full animate-spin inline-block" />
            Loading…
          </span>
        )}
        {!loading && events !== null && (
          <span className="text-xs text-text-tertiary">{events.length} events (live)</span>
        )}
        {!loading && events === null && (
          <span className="text-xs text-amber-400">Demo data</span>
        )}
      </div>

      <div className="bg-surface-base border border-border rounded-md overflow-hidden shadow-xs">
        {/* Header */}
        <div className="grid grid-cols-[1fr_1.5fr_2fr_auto] gap-4 px-4 h-9 items-center border-b border-border bg-surface-subtle">
          {['Source', 'Action', 'Message', 'Time'].map((h) => (
            <span key={h} className="text-xs font-semibold text-text-tertiary uppercase tracking-wide">{h}</span>
          ))}
        </div>

        {/* Rows */}
        {rows.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-text-tertiary">No audit events yet.</div>
        ) : (
          rows.map((row) => (
            <div
              key={row.id}
              className="grid grid-cols-[1fr_1.5fr_2fr_auto] gap-4 px-4 h-dp-row items-center border-b border-border last:border-0 hover:bg-surface-subtle transition-fast"
            >
              <span className="text-sm text-text-primary font-medium truncate">{row.user}</span>
              <Badge variant={ACTION_VARIANT[row.action] ?? 'neutral'}>{row.action}</Badge>
              <span className="text-sm text-text-secondary truncate" title={row.resource}>{row.resource}</span>
              <span className="text-sm text-text-tertiary tabular-nums shrink-0">{row.timestamp}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

const DEMO_ROWS = [
  { id: 0, user: 'admin',  action: 'vault.unlock',    resource: 'Vault unlocked (argon2id KDF)',  timestamp: '10:41:02' },
  { id: 1, user: 'john',   action: 'storage.read',    resource: 'ENTRY.READ entry#4a2f',          timestamp: '10:40:58' },
  { id: 2, user: 'admin',  action: 'storage.write',   resource: 'ENTRY.CREATE password entry',    timestamp: '10:38:14' },
  { id: 3, user: 'system', action: 'integrity.check', resource: 'Binary hash verified clean',     timestamp: '10:35:00' },
  { id: 4, user: 'john',   action: 'storage.delete',  resource: 'ENTRY.DELETE entry#3e7d',        timestamp: '10:30:11' },
]
