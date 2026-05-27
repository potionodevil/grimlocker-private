import { useState } from 'react'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'
import styles from './WorkspaceSwitcher.module.css'

export const WorkspaceSwitcher = ({ onWorkspaceSwitch }) => {
  const [isOpen, setIsOpen] = useState(false)
  const [isCreating, setIsCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const { workspaces, activeWorkspace } = useGrimStore()

  const handleCreateWorkspace = async (e) => {
    e.preventDefault()
    if (!newName.trim()) {
      setError('Workspace name required')
      return
    }

    setLoading(true)
    setError('')

    try {
      const result = await tauriBridge.createWorkspace(newName)
      setNewName('')
      setIsCreating(false)
      setIsOpen(false)
      if (onWorkspaceSwitch) {
        onWorkspaceSwitch()
      }
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
      await tauriBridge.switchWorkspace(workspaceId)
      setIsOpen(false)
      if (onWorkspaceSwitch) {
        onWorkspaceSwitch()
      }
    } catch (err) {
      setError(`Failed to switch workspace: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const handleDeleteWorkspace = async (workspaceId) => {
    if (workspaceId === 'default') {
      setError('Cannot delete default workspace')
      return
    }

    if (!window.confirm('Are you sure you want to delete this workspace?')) {
      return
    }

    setLoading(true)
    setError('')

    try {
      await tauriBridge.deleteWorkspace(workspaceId)
      setIsOpen(false)
      if (onWorkspaceSwitch) {
        onWorkspaceSwitch()
      }
    } catch (err) {
      setError(`Failed to delete workspace: ${err.message}`)
    } finally {
      setLoading(false)
    }
  }

  const currentName = activeWorkspace?.name || 'Workspaces'

  return (
    <div className={styles.container}>
      <button
        className={styles.trigger}
        onClick={() => setIsOpen(!isOpen)}
        title="Switch workspace"
      >
        <span className={styles.icon}>📦</span>
        <span className={styles.name}>{currentName}</span>
        <span className={`${styles.arrow} ${isOpen ? styles.open : ''}`}>▼</span>
      </button>

      {isOpen && (
        <div className={styles.dropdown}>
          <div className={styles.header}>
            <h3>Workspaces</h3>
            {error && <div className={styles.error}>{error}</div>}
          </div>

          <div className={styles.list}>
            {workspaces.map((ws) => (
              <div
                key={ws.id}
                className={`${styles.item} ${
                  ws.id === activeWorkspace?.id ? styles.active : ''
                }`}
              >
                <button
                  className={styles.itemButton}
                  onClick={() => handleSwitchWorkspace(ws.id)}
                  disabled={loading}
                >
                  <span className={styles.itemName}>{ws.name}</span>
                  {ws.is_default && <span className={styles.badge}>default</span>}
                </button>
                {ws.id !== 'default' && (
                  <button
                    className={styles.deleteBtn}
                    onClick={() => handleDeleteWorkspace(ws.id)}
                    disabled={loading}
                    title="Delete workspace"
                  >
                    ✕
                  </button>
                )}
              </div>
            ))}
          </div>

          <div className={styles.divider} />

          {!isCreating ? (
            <button
              className={styles.createBtn}
              onClick={() => setIsCreating(true)}
              disabled={loading}
            >
              + New Workspace
            </button>
          ) : (
            <form onSubmit={handleCreateWorkspace} className={styles.createForm}>
              <input
                type="text"
                placeholder="Workspace name"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                disabled={loading}
                autoFocus
                className={styles.createInput}
              />
              <div className={styles.createActions}>
                <button
                  type="submit"
                  disabled={loading || !newName.trim()}
                  className={styles.createSubmit}
                >
                  Create
                </button>
                <button
                  type="button"
                  onClick={() => {
                    setIsCreating(false)
                    setNewName('')
                  }}
                  disabled={loading}
                  className={styles.createCancel}
                >
                  Cancel
                </button>
              </div>
            </form>
          )}
        </div>
      )}
    </div>
  )
}
