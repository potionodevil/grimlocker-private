import { useState, useCallback } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { EntryCard } from './EntryCard'
import { EntryContextMenu } from './EntryContextMenu'

const FILTERS = [
  { id: 'all',       label: 'All' },
  { id: 'passwords', label: 'Passwords' },
  { id: 'ssh',       label: 'SSH Keys' },
  { id: 'certs',     label: 'Certificates' },
]

export function VaultGrid({ filter = 'all' }) {
  const entries = useGrimStore((s) => s.entries)
  const [listView, setListView]     = useState(false)
  const [activeFilter, setFilter]   = useState(filter)
  const [contextMenu, setContextMenu] = useState(null)

  const visible = entries.filter((e) =>
    activeFilter === 'all' || e.type === activeFilter
  )

  const handleContextMenu = useCallback((e, entry) => {
    e.preventDefault()
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      entry,
    })
  }, [])

  const handleCloseContextMenu = useCallback(() => {
    setContextMenu(null)
  }, [])

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-6 px-6 py-3 border-b border-border bg-surface-base shrink-0">
        <div className="flex items-center gap-1">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              onClick={() => setFilter(f.id)}
              className={[
                'px-3 h-7 rounded-md text-sm transition-fast',
                activeFilter === f.id
                  ? 'bg-accent-subtle text-accent font-medium'
                  : 'text-text-secondary hover:text-text-primary hover:bg-surface-subtle',
              ].join(' ')}
            >
              {f.label}
            </button>
          ))}
        </div>

        <div className="ml-auto flex items-center gap-1">
          <button
            onClick={() => setListView(false)}
            className={`w-7 h-7 flex items-center justify-center rounded transition-fast ${!listView ? 'bg-surface-subtle text-text-primary' : 'text-text-tertiary hover:text-text-primary'}`}
            title="Grid view"
          >
            \u229E
          </button>
          <button
            onClick={() => setListView(true)}
            className={`w-7 h-7 flex items-center justify-center rounded transition-fast ${listView ? 'bg-surface-subtle text-text-primary' : 'text-text-tertiary hover:text-text-primary'}`}
            title="List view"
          >
            \u2261
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto p-6">
        {visible.length === 0 ? (
          <EmptyState />
        ) : listView ? (
          <div className="bg-surface-base border border-border rounded-md overflow-hidden">
            {visible.map((e) => <EntryCard key={e.id} entry={e} listView onContextMenu={handleContextMenu} />)}
          </div>
        ) : (
          <div
            className="grid gap-dp-gap"
            style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}
          >
            {visible.map((e) => <EntryCard key={e.id} entry={e} onContextMenu={handleContextMenu} />)}
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

function EmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <div className="text-4xl mb-4">{'\uD83D\uDD12'}</div>
      <p className="text-base font-medium text-text-primary mb-1">No entries yet</p>
      <p className="text-sm text-text-secondary">Add your first password, SSH key, or certificate.</p>
    </div>
  )
}