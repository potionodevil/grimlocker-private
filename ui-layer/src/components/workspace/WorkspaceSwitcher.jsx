import { useState, useEffect, useRef } from 'react'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'
import { ConfirmDialog } from '../ui/ConfirmDialog'

export const WorkspaceSwitcher = ({ onWorkspaceSwitch }) => {
  const [isOpen, setIsOpen] = useState(false)
  const [isCreating, setIsCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  // Rename state: which workspace is being renamed inline
  const [renamingId, setRenamingId] = useState(null)
  const [renameValue, setRenameValue] = useState('')

  // Delete confirm dialog
  const [deleteConfirm, setDeleteConfirm] = useState(null) // workspace id to delete

  // Right-click context menu
  const [contextMenu, setContextMenu] = useState(null) // { x, y, workspace }

  const workspaces = useGrimStore((s) => s.workspaces)
  const activeWorkspace = useGrimStore((s) => s.activeWorkspace)
  const dropdownRef = useRef(null)
  const renameInputRef = useRef(null)

  useEffect(() => {
    if (!isOpen) return
    const handleClick = (e) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
        setIsOpen(false)
      }
    }
    const handleKey = (e) => {
      if (e.key === 'Escape' && !deleteConfirm) setIsOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
    }
  }, [isOpen, deleteConfirm])

  // Close context menu on any outside click
  useEffect(() => {
    if (!contextMenu) return
    const close = () => setContextMenu(null)
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [contextMenu])

  // Focus rename input when entering rename mode
  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [renamingId])

  const handleCreateWorkspace = async (e) => {
    e.preventDefault()
    if (!newName.trim()) {
      setError('Workspace name required')
      return
    }
    setLoading(true)
    setError('')
    try {
      await tauriBridge.createWorkspace(newName)
      setNewName('')
      setIsCreating(false)
      setIsOpen(false)
      if (onWorkspaceSwitch) onWorkspaceSwitch()
    } catch (err) {
      setError(`Failed to create workspace: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const handleSwitchWorkspace = async (workspaceId) => {
    if (workspaceId === activeWorkspace?.id) {
      setIsOpen(false)
      return
    }
    setLoading(true)
    setError('')
    try {
      const switched = await tauriBridge.switchWorkspace(workspaceId)
      setIsOpen(false)
      if (onWorkspaceSwitch) onWorkspaceSwitch(switched)
    } catch (err) {
      setError(`Failed to switch workspace: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const handleDeleteWorkspace = async () => {
    const workspaceId = deleteConfirm
    setDeleteConfirm(null)
    setLoading(true)
    setError('')
    try {
      await tauriBridge.deleteWorkspace(workspaceId)
      setIsOpen(false)
      if (onWorkspaceSwitch) onWorkspaceSwitch()
    } catch (err) {
      setError(`Failed to delete workspace: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const startRename = (ws) => {
    setRenamingId(ws.id)
    setRenameValue(ws.name)
  }

  const commitRename = async () => {
    if (!renameValue.trim() || !renamingId) {
      setRenamingId(null)
      return
    }
    setLoading(true)
    setError('')
    try {
      await tauriBridge.renameWorkspace(renamingId, renameValue.trim())
    } catch (err) {
      setError(`Failed to rename: ${err.message}`)
    } finally {
      setRenamingId(null)
      setLoading(false)
    }
  }

  const cancelRename = () => setRenamingId(null)

  const handleContextMenu = (e, ws) => {
    if (ws.is_default) return
    e.preventDefault()
    e.stopPropagation()
    setContextMenu({ x: e.clientX, y: e.clientY, workspace: ws })
  }

  const currentName = activeWorkspace?.name || 'Workspaces'

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={() => setIsOpen(!isOpen)}
        title="Switch workspace"
        className="flex items-center gap-2 px-3 h-8 rounded-md border border-border text-text-secondary text-sm font-medium hover:border-border-strong hover:text-text-primary transition-fast"
      >
        <span className="font-mono text-xs">&#9776;</span>
        <span className="max-w-[120px] truncate">{currentName}</span>
        <span className={`text-[10px] transition-transform ${isOpen ? 'rotate-180' : ''}`}>&#9662;</span>
      </button>

      {isOpen && (
        <div className="absolute top-full left-0 mt-1 bg-surface-base border border-border rounded-lg shadow-md z-50 min-w-[240px] max-w-[300px]">
          <div className="px-3 py-2.5 border-b border-border">
            <h3 className="text-xs font-mono text-accent uppercase tracking-wider">Workspaces</h3>
            <p className="text-[10px] text-text-tertiary mt-0.5">Double-click or right-click to rename</p>
            {error && (
              <div className="mt-2 px-2 py-1.5 bg-danger-subtle border border-danger rounded text-xs text-danger">{error}</div>
            )}
          </div>

          <div className="max-h-[300px] overflow-y-auto">
            {workspaces.map((ws) => (
              <div
                key={ws.id}
                className={`flex items-center border-b border-border last:border-b-0 ${
                  ws.id === activeWorkspace?.id ? 'bg-surface-subtle' : ''
                }`}
              >
                {renamingId === ws.id ? (
                  // Inline rename input
                  <div className="flex-1 px-2 py-1.5 flex gap-1">
                    <input
                      ref={renameInputRef}
                      type="text"
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') commitRename()
                        if (e.key === 'Escape') cancelRename()
                      }}
                      className="flex-1 h-7 px-2 rounded bg-surface-app border border-accent/60 text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent/40"
                    />
                    <button
                      onClick={commitRename}
                      className="h-7 px-2 rounded text-xs font-semibold text-white bg-accent hover:bg-accent-hover transition-fast"
                    >
                      OK
                    </button>
                    <button
                      onClick={cancelRename}
                      className="h-7 px-2 rounded text-xs text-text-secondary bg-surface-subtle hover:bg-border transition-fast"
                    >
                      &#x2715;
                    </button>
                  </div>
                ) : (
                  <button
                    onClick={() => handleSwitchWorkspace(ws.id)}
                    onDoubleClick={() => !ws.is_default && startRename(ws)}
                    onContextMenu={(e) => handleContextMenu(e, ws)}
                    disabled={loading}
                    title={ws.is_default ? ws.name : `Click to switch. Double-click or right-click to rename.`}
                    className="flex-1 px-3 py-2.5 text-left text-sm text-text-secondary hover:text-text-primary transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <span className="truncate">{ws.name}</span>
                    {ws.is_default && (
                      <span className="ml-2 px-1.5 py-0.5 rounded text-[10px] font-semibold bg-accent-subtle text-accent">
                        default
                      </span>
                    )}
                  </button>
                )}

                {renamingId !== ws.id && ws.id !== activeWorkspace?.id && !ws.is_default && (
                  <button
                    onClick={() => setDeleteConfirm(ws.id)}
                    disabled={loading}
                    title="Delete workspace"
                    className="px-2 py-2.5 text-text-tertiary hover:text-danger transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                  >
                    &#x2715;
                  </button>
                )}
              </div>
            ))}
          </div>

          <div className="border-t border-border">
            {!isCreating ? (
              <button
                onClick={() => setIsCreating(true)}
                disabled={loading}
                className="w-full px-3 py-2.5 text-xs font-semibold text-accent hover:bg-surface-subtle transition-colors disabled:opacity-50"
              >
                + New Workspace
              </button>
            ) : (
              <form onSubmit={handleCreateWorkspace} className="p-2 flex flex-col gap-1.5">
                <input
                  type="text"
                  placeholder="Workspace name"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  disabled={loading}
                  autoFocus
                  className="w-full h-8 px-2 rounded-md bg-surface-app border border-border text-sm text-text-primary placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent"
                />
                <div className="flex gap-1.5">
                  <button
                    type="submit"
                    disabled={loading || !newName.trim()}
                    className="flex-1 h-7 rounded text-xs font-semibold text-white bg-accent hover:bg-accent-hover disabled:opacity-50 disabled:cursor-not-allowed transition-fast"
                  >
                    Create
                  </button>
                  <button
                    type="button"
                    onClick={() => { setIsCreating(false); setNewName('') }}
                    disabled={loading}
                    className="flex-1 h-7 rounded text-xs font-semibold text-text-secondary bg-surface-subtle hover:bg-border transition-fast"
                  >
                    Cancel
                  </button>
                </div>
              </form>
            )}
          </div>
        </div>
      )}

      {/* Right-click context menu */}
      {contextMenu && (
        <div
          className="fixed z-[9999] bg-surface-base border border-border rounded-lg shadow-lg py-1 min-w-[140px]"
          style={{ top: contextMenu.y, left: contextMenu.x }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <button
            onClick={() => {
              startRename(contextMenu.workspace)
              setContextMenu(null)
              setIsOpen(true)
            }}
            className="w-full px-3 py-2 text-left text-sm text-text-secondary hover:text-text-primary hover:bg-surface-subtle transition-colors"
          >
            Umbenennen
          </button>
          {contextMenu.workspace.id !== activeWorkspace?.id && (
            <button
              onClick={() => {
                setDeleteConfirm(contextMenu.workspace.id)
                setContextMenu(null)
              }}
              className="w-full px-3 py-2 text-left text-sm text-danger hover:bg-danger-subtle transition-colors"
            >
              Loeschen
            </button>
          )}
        </div>
      )}

      {/* Delete confirmation dialog — rendered outside the dropdown for proper z-index */}
      <ConfirmDialog
        isOpen={!!deleteConfirm}
        title="Workspace loeschen"
        message="Dieser Workspace und alle enthaltenen Daten werden dauerhaft geloescht. Dieser Vorgang kann nicht rueckgaengig gemacht werden."
        confirmLabel="Workspace loeschen"
        cancelLabel="Abbrechen"
        variant="danger"
        onConfirm={handleDeleteWorkspace}
        onCancel={() => setDeleteConfirm(null)}
      />
    </div>
  )
}
