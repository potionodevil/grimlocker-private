import { useState, useCallback, useEffect } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { EntryCard } from './EntryCard'
import { EntryContextMenu } from './EntryContextMenu'
import { FileVaultUpload } from './FileVaultUpload'

// Filter definitions. `category` maps to the formal Category enum on the backend.
// Empty string means "all".
const FILTERS = [
  { id: 'all',        label: 'All',          category: '' },
  { id: 'passwords',  label: 'Passwords',    category: 'PASSWORD' },
  { id: 'ssh',        label: 'SSH Keys',     category: 'SSH_KEY' },
  { id: 'certs',      label: 'Certificates', category: 'CERTIFICATE' },
  { id: 'FILE_VAULT', label: 'Files',        category: 'FILE_VAULT' },
]

/**
 * Checks whether an entry matches a given filter.
 * Supports both the legacy `type` field and the formal `category` field.
 */
function entryMatchesFilter(entry, filter) {
  if (filter.category === '') return true
  if (entry.category === filter.category) return true
  // Legacy compatibility: map old type strings to categories
  const legacyMap = {
    password: 'PASSWORD',
    ssh: 'SSH_KEY',
    certs: 'CERTIFICATE',
    certificate: 'CERTIFICATE',
    file_vault: 'FILE_VAULT',
  }
  const mappedCategory = legacyMap[entry.type] || entry.type?.toUpperCase()
  return mappedCategory === filter.category
}

export function VaultGrid({ filter = 'all' }) {
  const entries = useGrimStore((s) => s.entries)
  const fetchEntries = useGrimStore((s) => s.fetchEntries)
  const [listView, setListView]       = useState(false)
  const [activeFilterId, setFilterId] = useState(filter)
  const [contextMenu, setContextMenu] = useState(null)
  const [showUpload, setShowUpload]   = useState(false)

  // Sync with the `filter` prop so sidebar navigation updates the active tab.
  useEffect(() => {
    setFilterId(filter)
    setShowUpload(false)
  }, [filter])

  const activeFilter = FILTERS.find((f) => f.id === activeFilterId) || FILTERS[0]

  const visible = entries.filter((e) => entryMatchesFilter(e, activeFilter))

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
        />
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
