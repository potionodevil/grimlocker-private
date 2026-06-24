import { useState, useCallback, useEffect, useRef } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import { EntryCard } from './EntryCard'
import { EntryContextMenu } from './EntryContextMenu'
import { FileVaultUpload } from './FileVaultUpload'
import { FileVaultViewer } from './FileVaultViewer'
import { EditEntryModal } from './EditEntryModal'

const FILTERS = [
  { id: 'all',       label: 'All',          category: '' },
  { id: 'passwords', label: 'Passwörter',   category: 'PASSWORD' },
  { id: 'ssh',       label: 'SSH Keys',     category: 'SSH_KEY' },
  { id: 'certs',     label: 'Zertifikate',  category: 'CERTIFICATE' },
]

const LEGACY_MAP = {
  password: 'PASSWORD', ssh: 'SSH_KEY',
  certs: 'CERTIFICATE', certificate: 'CERTIFICATE', file_vault: 'FILE_VAULT',
}

const SORT_OPTIONS = [
  { value: 'name',     label: 'Name' },
  { value: 'updated',  label: 'Geändert' },
  { value: 'created',  label: 'Erstellt' },
  { value: 'type',     label: 'Typ' },
  { value: 'strength', label: 'Stärke' },
]

function entryMatchesFilter(entry, filter) {
  const cat = entry.category || LEGACY_MAP[entry.type] || entry.type?.toUpperCase()
  if (filter.category === '') return cat !== 'FILE_VAULT'
  if (entry.category === filter.category) return true
  return cat === filter.category
}

function sortEntries(entries, sortBy, sortDir) {
  const dir = sortDir === 'asc' ? 1 : -1
  return [...entries].sort((a, b) => {
    let va, vb
    switch (sortBy) {
      case 'updated':  va = a.updatedAt ?? 0; vb = b.updatedAt ?? 0; break
      case 'created':  va = a.createdAt ?? 0; vb = b.createdAt ?? 0; break
      case 'type':     va = (a.category || a.type || '').toLowerCase(); vb = (b.category || b.type || '').toLowerCase(); break
      case 'strength': va = a.strength ?? 0; vb = b.strength ?? 0; break
      default:         va = (a.title || '').toLowerCase(); vb = (b.title || '').toLowerCase()
    }
    if (va < vb) return -dir
    if (va > vb) return dir
    return 0
  })
}

// ── Icons ─────────────────────────────────────────────────────────────────────
function IconGrid() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/>
      <rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/>
    </svg>
  )
}
function IconList() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/>
      <line x1="3" y1="18" x2="21" y2="18"/>
    </svg>
  )
}
function IconCompact() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
      <line x1="3" y1="5" x2="21" y2="5"/><line x1="3" y1="9" x2="21" y2="9"/>
      <line x1="3" y1="13" x2="21" y2="13"/><line x1="3" y1="17" x2="21" y2="17"/>
      <line x1="3" y1="21" x2="21" y2="21"/>
    </svg>
  )
}
function IconAsc() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <path d="M12 5v14M5 12l7-7 7 7"/>
    </svg>
  )
}
function IconDesc() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
      <path d="M12 19V5M5 12l7 7 7-7"/>
    </svg>
  )
}

export function VaultGrid({ filter = 'all', group = null }) {
  const entries        = useGrimStore((s) => s.entries)
  const fetchEntries   = useGrimStore((s) => s.fetchEntries)
  const connected      = useGrimStore((s) => s.daemonStatus)
  const prefs          = useGrimStore((s) => s.preferences)
  const setVaultViewMode = useGrimStore((s) => s.setVaultViewMode)
  const setVaultSortBy   = useGrimStore((s) => s.setVaultSortBy)
  const setVaultSortDir  = useGrimStore((s) => s.setVaultSortDir)

  const [activeFilterId, setFilterId]     = useState(filter)
  const [contextMenu, setContextMenu]     = useState(null)
  const [showUpload, setShowUpload]       = useState(false)
  const [editEntry, setEditEntry]         = useState(null)
  const [viewingFile, setViewingFile]     = useState(null)
  const [overwriteEntry, setOverwriteEntry] = useState(null)
  const fetchedOnce = useRef(false)

  const viewMode    = prefs.vaultViewMode  ?? 'grid'
  const sortBy      = prefs.vaultSortBy    ?? 'name'
  const sortDir     = prefs.vaultSortDir   ?? 'asc'
  const gridCardSize = prefs.gridCardSize  ?? 'default'

  const GRID_MIN = { small: '170px', default: '210px', large: '260px' }[gridCardSize] ?? '210px'

  useEffect(() => {
    if (connected === 'online' && !fetchedOnce.current) {
      fetchedOnce.current = true
      fetchEntries()
    }
  }, [connected, fetchEntries])

  useEffect(() => {
    const unsub = tauriBridge.on('connected', () => {
      fetchedOnce.current = true
      fetchEntries()
    })
    return unsub
  }, [fetchEntries])

  useEffect(() => {
    setFilterId(filter)
    setShowUpload(false)
  }, [filter])

  const activeFilter = FILTERS.find((f) => f.id === activeFilterId) || FILTERS[0]
  const isFileVault  = activeFilter.category === 'FILE_VAULT'

  const visible = sortEntries(
    entries.filter((e) => {
      if (!entryMatchesFilter(e, activeFilter)) return false
      if (group) {
        const eg = e.group || e.fields?.group || ''
        return eg === group
      }
      return true
    }),
    sortBy,
    sortDir,
  )

  const handleContextMenu = useCallback((e, entry) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, entry })
  }, [])

  const toggleSortDir = () => setVaultSortDir(sortDir === 'asc' ? 'desc' : 'asc')

  return (
    <div className="flex flex-col h-full">
      {/* ── Toolbar ───────────────────────────────────────────────────────── */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-border bg-surface-base shrink-0 flex-wrap">
        {/* Filter-Tabs */}
        <div className="flex items-center gap-0.5">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              onClick={() => { setFilterId(f.id); setShowUpload(false) }}
              className={[
                'px-3 h-7 rounded-md text-xs transition-fast',
                activeFilterId === f.id
                  ? 'bg-accent-subtle text-accent font-medium'
                  : 'text-text-secondary hover:text-text-primary hover:bg-surface-subtle',
              ].join(' ')}
            >
              {f.label}
            </button>
          ))}
        </div>

        <div className="flex-1" />

        {/* ── Sort-Dropdown + Richtung ─────────────────────────────────── */}
        {!isFileVault && (
          <div className="flex items-center gap-1">
            <select
              value={sortBy}
              onChange={(e) => setVaultSortBy(e.target.value)}
              className="h-7 px-2 text-xs rounded-md bg-surface-subtle border border-border text-text-secondary hover:text-text-primary focus:outline-none focus:ring-1 focus:ring-accent cursor-pointer appearance-none pr-6"
              style={{ backgroundImage: "url(\"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6'%3E%3Cpath d='M0 0l5 6 5-6z' fill='%236b7280'/%3E%3C/svg%3E\")", backgroundRepeat: 'no-repeat', backgroundPosition: 'right 6px center' }}
            >
              {SORT_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>{o.label}</option>
              ))}
            </select>
            <button
              onClick={toggleSortDir}
              title={sortDir === 'asc' ? 'Aufsteigend' : 'Absteigend'}
              className="w-7 h-7 flex items-center justify-center rounded-md text-text-tertiary hover:text-text-primary hover:bg-surface-subtle transition-fast border border-border"
            >
              {sortDir === 'asc' ? <IconAsc /> : <IconDesc />}
            </button>
          </div>
        )}

        {/* ── Upload-Button (FILE_VAULT) ─────────────────────────────────── */}
        {isFileVault && (
          <button
            onClick={() => setShowUpload((v) => !v)}
            className={[
              'h-7 px-3 rounded-md text-xs transition-fast',
              showUpload
                ? 'bg-accent text-white'
                : 'bg-surface-subtle text-text-secondary hover:text-text-primary',
            ].join(' ')}
          >
            ⬆ Upload
          </button>
        )}

        {/* ── View-Mode-Toggle ──────────────────────────────────────────── */}
        {!isFileVault && (
          <div className="flex items-center gap-0.5 p-0.5 bg-surface-subtle rounded-md border border-border">
            {[
              { mode: 'grid',    Icon: IconGrid,    title: 'Grid-Ansicht' },
              { mode: 'list',    Icon: IconList,    title: 'Listen-Ansicht' },
              { mode: 'compact', Icon: IconCompact, title: 'Kompakt-Ansicht' },
            ].map(({ mode, Icon: Ic, title }) => (
              <button
                key={mode}
                onClick={() => setVaultViewMode(mode)}
                title={title}
                className={[
                  'w-6 h-6 flex items-center justify-center rounded transition-fast',
                  viewMode === mode
                    ? 'bg-surface-base text-text-primary shadow-sm'
                    : 'text-text-tertiary hover:text-text-secondary',
                ].join(' ')}
              >
                <Ic />
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Upload-Panel */}
      {isFileVault && showUpload && (
        <div className="px-6 py-4 border-b border-border bg-surface-subtle shrink-0">
          <FileVaultUpload onSuccess={() => { fetchEntries(); setShowUpload(false) }} />
        </div>
      )}

      {/* ── Entry-Inhalt ──────────────────────────────────────────────────── */}
      <div className="flex-1 overflow-y-auto">
        {visible.length === 0 ? (
          <EmptyState isFileVault={isFileVault} onUpload={() => setShowUpload(true)} />
        ) : viewMode === 'grid' && !isFileVault ? (
          <div className="p-4 grid gap-3" style={{ gridTemplateColumns: `repeat(auto-fill, minmax(${GRID_MIN}, 1fr))` }}>
            {visible.map((e) => (
              <EntryCard key={e.id} entry={e} viewMode="grid" onContextMenu={handleContextMenu} />
            ))}
          </div>
        ) : viewMode === 'compact' && !isFileVault ? (
          <div className="py-1">
            {/* Spalten-Header für Compact */}
            <div className="flex items-center gap-3 px-4 py-1 border-b border-border text-[10px] font-medium text-text-tertiary uppercase tracking-wider">
              <span className="w-12 shrink-0">Typ</span>
              <span className="flex-1">Name</span>
              <span className="w-28 text-right">Geändert</span>
            </div>
            {visible.map((e) => (
              <EntryCard key={e.id} entry={e} viewMode="compact" onContextMenu={handleContextMenu} />
            ))}
          </div>
        ) : (
          /* List mode (+ FileVault) */
          <div>
            {!isFileVault && (
              <div className="flex items-center gap-4 pl-3 pr-4 py-1.5 border-b border-border text-[10px] font-medium text-text-tertiary uppercase tracking-wider sticky top-0 bg-surface-base z-10">
                <span className="w-14 shrink-0 pl-1">Typ</span>
                <span className="flex-1">Name</span>
                <span className="w-16 shrink-0">Stärke</span>
                <span className="w-16 text-right">Geändert</span>
              </div>
            )}
            {visible.map((e, i) => (
              <EntryCard key={e.id} entry={e} viewMode="list" onContextMenu={handleContextMenu} index={i} />
            ))}
          </div>
        )}
      </div>

      {contextMenu && (
        <EntryContextMenu
          entry={contextMenu.entry}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
          onEdit={(entry) => setEditEntry(entry)}
          onOpenFile={(entry) => { setViewingFile(entry); setContextMenu(null) }}
          onOverwrite={(entry) => { setOverwriteEntry(entry); setContextMenu(null) }}
        />
      )}

      {editEntry && (
        <EditEntryModal
          entry={editEntry}
          open={!!editEntry}
          onClose={() => setEditEntry(null)}
          onSaved={() => { fetchEntries(); setEditEntry(null) }}
        />
      )}

      {viewingFile && (
        <FileVaultViewer
          entry={viewingFile}
          isOpen
          onClose={() => setViewingFile(null)}
        />
      )}

      {overwriteEntry && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-surface-base border border-border rounded-lg shadow-lg p-6 w-full max-w-md">
            <h3 className="text-sm font-semibold text-text-primary mb-1">Datei überschreiben</h3>
            <p className="text-xs text-text-tertiary mb-4 truncate">{overwriteEntry.fields?.file_name || overwriteEntry.title}</p>
            <FileVaultUpload
              onSuccess={async (newManifest) => {
                try {
                  await tauriBridge.updateEntry(overwriteEntry.id, {
                    ...overwriteEntry.fields,
                    manifest_block_id: newManifest.manifest_block_id || newManifest.id,
                    file_name: newManifest.file_name,
                    mime_type: newManifest.mime_type,
                    total_size: String(newManifest.total_size),
                  })
                } catch (err) {
                  console.warn('[VaultGrid] overwrite updateEntry failed:', err.message)
                }
                fetchEntries()
                setOverwriteEntry(null)
              }}
              onCancel={() => setOverwriteEntry(null)}
            />
          </div>
        </div>
      )}
    </div>
  )
}

function EmptyState({ isFileVault, onUpload }) {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <div className="text-4xl mb-4 text-text-disabled font-mono select-none">
        {isFileVault ? '▤' : '·'}
      </div>
      <p className="text-base font-medium text-text-primary mb-1">
        {isFileVault ? 'Keine Dateien' : 'Keine Einträge'}
      </p>
      <p className="text-sm text-text-secondary mb-4">
        {isFileVault
          ? 'Lade Dateien hoch um sie sicher im Vault zu speichern.'
          : 'Füge dein erstes Passwort, SSH-Key oder Zertifikat hinzu.'}
      </p>
      {isFileVault && (
        <button
          onClick={onUpload}
          className="h-9 px-4 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover transition-fast"
        >
          Datei hochladen
        </button>
      )}
    </div>
  )
}
