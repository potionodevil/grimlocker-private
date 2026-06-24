import { useState, useEffect } from 'react'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * HistoryDrawer — Side-Drawer der die Versionshistorie eines Vault-Eintrags zeigt.
 * Props:
 *   entry  — {id, title, ...} — der aktuell ausgewählte Entry
 *   onClose — wird gerufen wenn der Drawer geschlossen werden soll
 *   onRestored — wird gerufen nachdem eine Version erfolgreich wiederhergestellt wurde
 */
export function HistoryDrawer({ entry, onClose, onRestored }) {
  const [snapshots, setSnapshots] = useState([])
  const [loading, setLoading]     = useState(false)
  const [error, setError]         = useState(null)
  const [selected, setSelected]   = useState(null) // SnapMeta | null
  const [restoring, setRestoring] = useState(false)
  const [restored, setRestored]   = useState(false)

  useEffect(() => {
    if (!entry?.id) return
    setLoading(true)
    setError(null)
    setSnapshots([])
    setSelected(null)
    setRestored(false)
    tauriBridge.getEntryHistory(entry.id)
      .then((r) => setSnapshots(r.snapshots ?? []))
      .catch((e) => setError(e?.message ?? 'Verlauf konnte nicht geladen werden.'))
      .finally(() => setLoading(false))
  }, [entry?.id])

  const handleRestore = async () => {
    if (!selected) return
    setRestoring(true)
    setError(null)
    try {
      await tauriBridge.restoreEntryVersion(entry.id, selected.snap_id)
      setRestored(true)
      onRestored?.()
    } catch (e) {
      setError(e?.message ?? 'Wiederherstellen fehlgeschlagen.')
    } finally {
      setRestoring(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/40" onClick={onClose} />

      {/* Drawer */}
      <div className="relative flex flex-col w-80 h-full bg-surface-base border-l border-border shadow-xl overflow-hidden">
        {/* Header */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-border shrink-0">
          <button
            onClick={onClose}
            className="w-7 h-7 rounded-lg hover:bg-surface-subtle flex items-center justify-center text-text-tertiary transition-fast"
            aria-label="Schließen"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
              <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
            </svg>
          </button>
          <div className="min-w-0">
            <p className="text-sm font-semibold text-text-primary truncate">Versionshistorie</p>
            <p className="text-[11px] text-text-tertiary truncate">{entry?.title}</p>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 overflow-y-auto">
          {loading && (
            <div className="flex items-center justify-center py-12 text-text-tertiary text-sm">
              <span className="w-4 h-4 border-2 border-current/30 border-t-current rounded-full animate-spin mr-2" />
              Lade Verlauf…
            </div>
          )}

          {!loading && error && (
            <div className="m-4 px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">
              {error}
            </div>
          )}

          {!loading && !error && snapshots.length === 0 && (
            <div className="flex flex-col items-center gap-2 py-12 text-center px-4">
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} className="text-text-tertiary">
                <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
              </svg>
              <p className="text-sm text-text-secondary">Keine früheren Versionen</p>
              <p className="text-xs text-text-tertiary">Versionen werden beim Bearbeiten automatisch gespeichert.</p>
            </div>
          )}

          {!loading && snapshots.length > 0 && (
            <ul className="divide-y divide-border">
              {snapshots.map((snap) => (
                <SnapItem
                  key={snap.snap_id}
                  snap={snap}
                  selected={selected?.snap_id === snap.snap_id}
                  onSelect={() => setSelected(snap)}
                />
              ))}
            </ul>
          )}
        </div>

        {/* Preview + Restore */}
        {selected && (
          <div className="border-t border-border shrink-0 p-4 space-y-3">
            <SnapPreview snap={selected} />
            {restored ? (
              <div className="px-3 py-2 rounded-lg bg-green-500/10 border border-green-500/30 text-green-400 text-xs text-center">
                Version wiederhergestellt ✓
              </div>
            ) : (
              <button
                onClick={handleRestore}
                disabled={restoring}
                className="w-full h-8 rounded-lg text-xs font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-50 disabled:cursor-not-allowed transition-fast flex items-center justify-center gap-1.5"
              >
                {restoring ? (
                  <><span className="w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" /> Wiederherstellung…</>
                ) : (
                  <>
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5}>
                      <polyline points="1 4 1 10 7 10"/><path d="M1 10a9 9 0 1 0 9-9"/>
                    </svg>
                    Diese Version wiederherstellen
                  </>
                )}
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

function SnapItem({ snap, selected, onSelect }) {
  const date = new Date(snap.ts / 1_000_000) // nanoseconds → ms
  const label = formatRelative(date)
  const abs   = date.toLocaleString('de-DE', { dateStyle: 'short', timeStyle: 'short' })

  return (
    <li>
      <button
        onClick={onSelect}
        className={`w-full text-left px-4 py-3 flex items-center gap-3 transition-fast ${
          selected ? 'bg-accent/10 border-r-2 border-accent' : 'hover:bg-surface-subtle'
        }`}
      >
        <div className="w-8 h-8 rounded-lg bg-surface-subtle flex items-center justify-center shrink-0 text-text-tertiary">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
            <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
          </svg>
        </div>
        <div className="min-w-0">
          <p className="text-xs font-medium text-text-primary">{label}</p>
          <p className="text-[10px] text-text-tertiary">{abs}</p>
        </div>
        {selected && (
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} className="ml-auto text-accent shrink-0">
            <polyline points="20 6 9 17 4 12"/>
          </svg>
        )}
      </button>
    </li>
  )
}

function SnapPreview({ snap }) {
  let fields = {}
  try {
    const parsed = JSON.parse(new TextDecoder().decode(
      typeof snap.data === 'string'
        ? Uint8Array.from(atob(snap.data), (c) => c.charCodeAt(0))
        : new Uint8Array(snap.data)
    ))
    fields = parsed?.fields ?? {}
  } catch {
    // data may be plaintext JSON already
    try { fields = JSON.parse(snap.data)?.fields ?? {} } catch { /* ignore */ }
  }

  const entries = Object.entries(fields).slice(0, 4)
  if (entries.length === 0) return null

  return (
    <div className="bg-surface-subtle rounded-lg p-3 space-y-1.5">
      <p className="text-[10px] font-medium text-text-tertiary uppercase tracking-wider mb-2">Vorschau</p>
      {entries.map(([k, v]) => (
        <div key={k} className="flex items-center gap-2 text-xs">
          <span className="text-text-tertiary w-20 truncate capitalize">{k}</span>
          <span className="text-text-secondary truncate">
            {k === 'password' ? '•'.repeat(Math.min(v.length, 12)) : v}
          </span>
        </div>
      ))}
    </div>
  )
}

function formatRelative(date) {
  const diff = Date.now() - date.getTime()
  const s = Math.floor(diff / 1000)
  if (s < 60)  return 'Gerade eben'
  const m = Math.floor(s / 60)
  if (m < 60)  return `Vor ${m} Minute${m > 1 ? 'n' : ''}`
  const h = Math.floor(m / 60)
  if (h < 24)  return `Vor ${h} Stunde${h > 1 ? 'n' : ''}`
  const d = Math.floor(h / 24)
  if (d < 7)   return `Vor ${d} Tag${d > 1 ? 'en' : ''}`
  return date.toLocaleDateString('de-DE')
}
