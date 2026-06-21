import { useEffect, useRef, useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { ConfirmDialog } from '../ui/ConfirmDialog'

const MENU_ITEMS = [
  { id: 'reveal',    label: 'Reveal',          icon: '\uD83D\uDD13', danger: false },
  { id: 'edit',      label: 'Edit',             icon: '\u270F',       danger: false },
  { id: 'open',      label: 'Open',             icon: '\u25B6',       danger: false },
  { id: 'overwrite', label: 'Ueberschreiben',   icon: '\u21A9',       danger: false, fileOnly: true },
  { id: 'delete',    label: 'Secure Delete',    icon: '\uD83D\uDDD1', danger: true },
]

export function EntryContextMenu({ entry, x, y, onClose, onEdit, onOpenFile, onOverwrite }) {
  const { fetchEntry, decryptEntry, deleteEntryFromStore, setActiveEntry } = useGrimStore()
  const menuRef = useRef(null)
  const [isDeleting, setIsDeleting] = useState(false)
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  useEffect(() => {
    const handleClickOutside = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        // Nicht schliessen, wenn der Confirm-Dialog offen ist — sonst verliert der User die Auswahl
        if (!confirmDeleteOpen) onClose()
      }
    }
    const handleEscape = (e) => {
      if (e.key === 'Escape' && !confirmDeleteOpen) onClose()
    }
    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscape)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [onClose, confirmDeleteOpen])

  const handleAction = async (actionId) => {
    switch (actionId) {
      case 'reveal':
        await decryptEntry(entry.id)
        onClose()
        break
      case 'edit':
        onClose()
        onEdit?.(entry)
        break
      case 'open':
        if (entry.category === 'FILE_VAULT' || entry.type === 'file_vault') {
          onClose()
          onOpenFile?.(entry)
        } else {
          await fetchEntry(entry.id)
          onClose()
        }
        break
      case 'overwrite':
        onClose()
        onOverwrite?.(entry)
        break
      case 'delete':
        setConfirmDeleteOpen(true)
        break
    }
  }

  const handleConfirmDelete = async () => {
    setConfirmDeleteOpen(false)
    setIsDeleting(true)
    await deleteEntryFromStore(entry.id)
    onClose()
  }

  const menuStyle = {
    position: 'fixed',
    left: x,
    top: y,
    zIndex: 50,
  }

  return (
    <>
      <div
        ref={menuRef}
        style={menuStyle}
        className="bg-surface-base border border-border rounded-lg shadow-lg py-1 min-w-[180px]"
      >
        {MENU_ITEMS.filter(item => !item.fileOnly || entry.category === 'FILE_VAULT' || entry.type === 'file_vault').map((item) => (
          <button
            key={item.id}
            onClick={() => handleAction(item.id)}
            disabled={isDeleting && item.id === 'delete'}
            className={[
              'w-full flex items-center gap-3 px-4 py-2 text-left text-sm transition-fast',
              item.danger
                ? 'text-danger hover:bg-danger-subtle'
                : 'text-text-primary hover:bg-surface-subtle',
              isDeleting && item.id === 'delete' ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
            ].join(' ')}
          >
            <span className="w-5 text-center">{item.icon}</span>
            <span>{item.label}</span>
            {isDeleting && item.id === 'delete' && (
              <span className="text-xs text-text-tertiary ml-auto">Deleting...</span>
            )}
          </button>
        ))}
      </div>

      <ConfirmDialog
        isOpen={confirmDeleteOpen}
        title="Eintrag sicher loeschen"
        message="Dieser Vorgang kann nicht rueckgaengig gemacht werden. Der Eintrag wird dauerhaft aus dem Vault entfernt."
        confirmLabel="Sicher loeschen"
        cancelLabel="Abbrechen"
        variant="danger"
        onConfirm={handleConfirmDelete}
        onCancel={() => setConfirmDeleteOpen(false)}
      />
    </>
  )
}