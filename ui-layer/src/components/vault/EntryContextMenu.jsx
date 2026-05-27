import { useEffect, useRef, useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'

const MENU_ITEMS = [
  { id: 'reveal', label: 'Reveal', icon: '\uD83D\uDD13', danger: false },
  { id: 'open', label: 'Open', icon: '\u25B6', danger: false },
  { id: 'delete', label: 'Secure Delete', icon: '\uD83D\uDDD1', danger: true },
]

export function EntryContextMenu({ entry, x, y, onClose }) {
  const { fetchEntry, decryptEntry, deleteEntryFromStore, setActiveEntry } = useGrimStore()
  const menuRef = useRef(null)
  const [isDeleting, setIsDeleting] = useState(false)

  useEffect(() => {
    const handleClickOutside = (e) => {
      if (menuRef.current && !menuRef.current.contains(e.target)) {
        onClose()
      }
    }
    const handleEscape = (e) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscape)
    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape)
    }
  }, [onClose])

  const handleAction = async (actionId) => {
    switch (actionId) {
      case 'reveal':
        await decryptEntry(entry.id)
        onClose()
        break
      case 'open':
        await fetchEntry(entry.id)
        onClose()
        break
      case 'delete':
        setIsDeleting(true)
        await deleteEntryFromStore(entry.id)
        onClose()
        break
    }
  }

  const menuStyle = {
    position: 'fixed',
    left: x,
    top: y,
    zIndex: 50,
  }

  return (
    <div
      ref={menuRef}
      style={menuStyle}
      className="bg-surface-base border border-border rounded-lg shadow-lg py-1 min-w-[180px]"
    >
      {MENU_ITEMS.map((item) => (
        <button
          key={item.id}
          onClick={() => handleAction(item.id)}
          disabled={isDeleting && item.id === 'delete'}
          className={[
            'w-full flex items-center gap-3 px-4 py-2 text-left text-sm transition-fast',
            item.danger
              ? 'text-red-400 hover:bg-red-500/10'
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
  )
}