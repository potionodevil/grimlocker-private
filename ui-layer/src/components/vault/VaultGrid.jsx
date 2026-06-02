import { useState, useCallback, useEffect, useRef } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import { EntryCard } from './EntryCard'
import { EntryContextMenu } from './EntryContextMenu'
import { FileVaultUpload } from './FileVaultUpload'
import { FileVaultViewer } from './FileVaultViewer'
import { EditEntryModal } from './EditEntryModal'

// Filter definitions. FILE_VAULT is handled by its own dedicated FileVaultBrowser.
const FILTERS = [
  { id: 'all',        label: 'All',          category: '' },
  { id: 'passwords',  label: 'Passwords',    category: 'PASSWORD' },
  { id: 'ssh',        label: 'SSH Keys',     category: 'SSH_KEY' },
  { id: 'certs',      label: 'Certificates', category: 'CERTIFICATE' },
]

const LEGACY_MAP = {
  password: 'PASSWORD',
  ssh: 'SSH_KEY',
  certs: 'CERTIFICATE',
  certificate: 'CERTIFICATE',
  file_vault: 'FILE_VAULT',
}

/**
 * Checks whether an entry matches a given filter.
 * "All Items" (category='') excludes FILE_VAULT — files live in FileVaultBrowser.
 */
function entryMatchesFilter(entry, filter) {
  const cat = entry.category || LEGACY_MAP[entry.type] || entry.type?.toUpperCase()
  if (filter.category === '') return cat !== 'FILE_VAULT'
  if (entry.category === filter.category) return true
  return cat === filter.category
}

export function VaultGrid({ filter = 'all', group = null }) {
  const entries = useGrimStore((s) => s.entries)
  const fetchEntries = useGrimStore((s) => s.fetchEntries)
  const connected = useGrimStore((s) => s.daemonStatus)
  const [listView, setListView]       = useState(false)
  const [activeFilterId, setFilterId] = useState(filter)
  const [contextMenu, setContextMenu]     = useState(null)
  const [showUpload, setShowUpload]       = useState(false)
  const [editEntry, setEditEntry]         = useState(null)
  const [viewingFile, setViewingFile]     = useState(null) // entry being viewed in FileVaultViewer
  const [overwriteEntry, setOverwriteEntry] = useState(null) // file entry being overwritten
  const fetchedOnce = useRef(false)

  useEffect(() => {
    if (connected === 'online' && !fetchedOnce.current) {
      fetchedOnce.current = true
      fetchEntries()
    }
  }, [connected, fetchEntries])

  // Re-fetch entries on reconnect
  useEffect(() => {
    const unsub = tauriBridge.on('connected', () => {
      fetchedOnce.current = true
      fetchEntries()
    })
    return unsub
  }, [fetchEntries])

  // Sync with the `filter` prop so sidebar navigation updates the active tab.
  useEffect(() => {
    setFilterId(filter)
    setShowUpload(false)
  }, [filter])

  const activeFilter = FILTERS.find((f) => f.id === activeFilterId) || FILTERS[0]

  const visible = entries.filter((e) => {
    if (!entryMatchesFilter(e, activeFilter)) return false
    if (group) {
      const entryGroup = e.group || e.fields?.group || ''
      return entryGroup === group
    }
    return true
  })

  const handleContextMenu = useCallback((e, entry) => {
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, entry })
  }, [])

  const handleCloseContextMenu = useCallback(() => {
    setContextMenu(null)
  }, [])

  const isFileVault = activeFilter.category === 'FILE_VAULT'

  return (
    <div className="flex flex-col h-full">
      {/* Filter bar */}
      <div className="flex items-center gap-6 px-6 py-3 border-b border-border bg-surface-base shrink-0">
        <div className="flex items-center gap-1">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              onClick={() => { setFilterId(f.id); setShowUpload(false) }}
              className={[
                'px-3 h-7 rounded-md text-sm transition-fast',
                activeFilterId === f.id
                  ? 'bg-accent-subtle text-accent font-medium'
                  : 'text-text-secondary hover:text-text-primary hover:bg-surface-subtle',
              ].join(' ')}
            >
              {f.label}
            </button>
          ))}
        </div>

        <div className="ml-auto flex items-center gap-1">
          {/* Upload button — only visible in FILE_VAULT tab */}
          {isFileVault && (
            <button
              onClick={() => setShowUpload((v) => !v)}
              className={[
                'h-7 px-3 rounded-md text-sm transition-fast',
                showUpload
                  ? 'bg-accent text-white'
                  : 'bg-surface-subtle text-text-secondary hover:text-text-primary',
              ].join(' ')}
              title="Upload file to vault"
            >
              ⬆ Upload
            </button>
          )}

          {/* View toggles */}
          {!isFileVault && (
            <>
              <button
                onClick={() => setListView(false)}
                className={`w-7 h-7 flex items-center justify-center rounded transition-fast ${!listView ? 'bg-surface-subtle text-text-primary' : 'text-text-tertiary hover:text-text-primary'}`}
                title="Grid view"
              >
                ⊞
              </button>
              <button
                onClick={() => setListView(true)}
                className={`w-7 h-7 flex items-center justify-center rounded transition-fast ${listView ? 'bg-surface-subtle text-text-primary' : 'text-text-tertiary hover:text-text-primary'}`}
                title="List view"
              >
                ≡
              </button>
            </>
          )}
        </div>
      </div>

      {/* File upload panel (collapsible, only in FILE_VAULT tab) */}
      {isFileVault && showUpload && (
        <div className="px-6 py-4 border-b border-border bg-surface-subtle shrink-0">
          <FileVaultUpload onSuccess={() => { fetchEntries(); setShowUpload(false) }} />
        </div>
      )}

      {/* Entry list / grid */}
      <div className="flex-1 overflow-y-auto p-6">
        {visible.length === 0 ? (
          <EmptyState isFileVault={isFileVault} onUpload={() => setShowUpload(true)} />
        ) : listView || isFileVault ? (
          <div className="bg-surface-base border border-border rounded-md overflow-hidden">
            {visible.map((e) => (
              <EntryCard key={e.id} entry={e} listView onContextMenu={handleContextMenu} />
            ))}
          </div>
        ) : (
          <div
            className="grid gap-dp-gap"
            style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}
          >
            {visible.map((e) => (
              <EntryCard key={e.id} entry={e} onContextMenu={handleContextMenu} />
            ))}
          </div>
        )}
      </div>

      {contextMenu && (
        <EntryContextMenu
          entry={contextMenu.entry}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={handleCloseContextMenu}
          onEdit={(entry) => setEditEntry(entry)}
          onOpenFile={(entry) => { setViewingFile(entry); handleCloseContextMenu() }}
          onOverwrite={(entry) => { setOverwriteEntry(entry); handleCloseContextMenu() }}
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

      {/* FileVaultViewer — opens when user right-clicks a file entry and picks "Open" */}
      {viewingFile && (
        <FileVaultViewer
          entry={viewingFile}
          isOpen={true}
          onClose={() => setViewingFile(null)}
        />
      )}

      {/* Overwrite modal — re-uses FileVaultUpload, updates the entry fields with new manifest */}
      {overwriteEntry && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-surface-base border border-border rounded-lg shadow-lg p-6 w-full max-w-md">
            <h3 className="text-sm font-semibold text-text-primary mb-1">Datei ueberschreiben</h3>
            <p className="text-xs text-text-tertiary mb-4 truncate">{overwriteEntry.fields?.file_name || overwriteEntry.title}</p>
            <FileVaultUpload
              onSuccess={async (newManifest) => {
                try {
                  await tauriBridge.updateEntry(overwriteEntry.id, {
                    ...overwriteEntry.fields,
                    manifest_block_id: newManifest.manifest_block_id || newManifest.id,
                    file_name:         newManifest.file_name,
                    mime_type:         newManifest.mime_type,
                    total_size:        String(newManifest.total_size),
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
        {isFileVault ? 'No files stored yet' : 'No entries yet'}
      </p>
      <p className="text-sm text-text-secondary mb-4">
        {isFileVault
          ? 'Upload files to encrypt and store them securely in your vault.'
          : 'Add your first password, SSH key, or certificate.'}
      </p>
      {isFileVault && (
        <button
          onClick={onUpload}
          className="h-9 px-4 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover transition-fast"
        >
          Upload File
        </button>
      )}
    </div>
  )
}
