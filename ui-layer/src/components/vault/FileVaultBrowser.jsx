import { useState, useCallback, useEffect, useRef } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { tauriBridge } from '../../services/tauriBridge'
import { FileVaultViewer } from './FileVaultViewer'
import { FileVaultUpload } from './FileVaultUpload'

/**
 * FileVaultBrowser — Hierarchischer Datei-Browser für den verschlüsselten FileVault.
 *
 * Zeigt Ordner + Dateien in einem zweispaltigen Layout:
 *   Links  — Breadcrumb-Navigation
 *   Rechts — Inhalt des ausgewählten Ordners (Dateien + Unterordner)
 *
 * User können: Ordner erstellen, Dateien hochladen, öffnen, umbenennen/löschen, verschieben.
 */
export function FileVaultBrowser({ jumpToFolder, onFolderChange, onRootFoldersChange }) {
  const [currentFolder, setCurrentFolder]  = useState(jumpToFolder ?? '')
  const [breadcrumbs, setBreadcrumbs]      = useState([])    // [{id, name}]
  const [contents, setContents]            = useState({ folders: [], files: [] })
  const [loading, setLoading]              = useState(false)
  const [error, setError]                  = useState(null)
  const [viewingFile, setViewingFile]      = useState(null)  // file entry for FileVaultViewer
  const [showUpload, setShowUpload]        = useState(false)
  const [contextMenu, setContextMenu]      = useState(null)  // {x, y, item}
  const [renaming, setRenaming]            = useState(null)  // {id, name, type}
  const [newFolderMode, setNewFolderMode]  = useState(false)
  const [newFolderName, setNewFolderName]  = useState('')
  const renameInputRef = useRef(null)
  const newFolderInputRef = useRef(null)

  const loadFolder = useCallback(async (folderId) => {
    setLoading(true)
    setError(null)
    try {
      const result = await tauriBridge.listFolder(folderId)
      setContents({
        folders: result.folders || [],
        files:   result.files   || [],
      })
      // Breadcrumb-Name updaten, falls es ein Platzhalter war (z.B. beim Sidebar-Sprung)
      if (result.name && folderId !== '') {
        setBreadcrumbs(prev => prev.map(b => b.id === folderId && b.name === '…' ? { ...b, name: result.name } : b))
      }
    } catch (err) {
      setError(err.message || 'Fehler beim Laden')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    loadFolder(currentFolder)
  }, [currentFolder, loadFolder])

  // Wenn Root-Ordner geladen wurden, an AppShell melden (für die Sidebar)
  useEffect(() => {
    if (currentFolder === '' && onRootFoldersChange) {
      tauriBridge.listFolder('').then(r => {
        onRootFoldersChange(r.folders || [])
      }).catch(() => {})
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentFolder])

  // Von der Sidebar in einen Ordner springen
  useEffect(() => {
    if (jumpToFolder === undefined) return
    const id = jumpToFolder ?? ''
    if (id !== currentFolder) {
      setCurrentFolder(id)
      if (id === '') {
        setBreadcrumbs([])
      } else {
        setBreadcrumbs([{ id, name: '…' }])
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [jumpToFolder])

  useEffect(() => {
    if (renaming && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [renaming])

  useEffect(() => {
    if (newFolderMode && newFolderInputRef.current) {
      newFolderInputRef.current.focus()
    }
  }, [newFolderMode])

  // Context-Menu schliessen bei Klick ausserhalb
  useEffect(() => {
    if (!contextMenu) return
    const close = (e) => {
      if (!e.target.closest('[data-context-menu]')) setContextMenu(null)
    }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [contextMenu])

  const navigateTo = (folderId, folderName) => {
    setCurrentFolder(folderId)
    if (folderId === '') {
      setBreadcrumbs([])
    } else {
      const idx = breadcrumbs.findIndex(b => b.id === folderId)
      if (idx >= 0) {
        setBreadcrumbs(prev => prev.slice(0, idx + 1))
      } else {
        setBreadcrumbs(prev => [...prev, { id: folderId, name: folderName }])
      }
    }
    setContextMenu(null)
  }

  const handleCreateFolder = async () => {
    const name = newFolderName.trim()
    if (!name) { setNewFolderMode(false); return }
    try {
      await tauriBridge.createFolder(name, currentFolder)
      setNewFolderMode(false)
      setNewFolderName('')
      await loadFolder(currentFolder)
    } catch (err) {
      setError(err.message)
    }
  }

  const handleRenameCommit = async () => {
    if (!renaming || !renaming.name.trim()) { setRenaming(null); return }
    try {
      if (renaming.type === 'folder') {
        await tauriBridge.renameFolder(renaming.id, renaming.name.trim())
      }
      setRenaming(null)
      await loadFolder(currentFolder)
    } catch (err) {
      setError(err.message)
      setRenaming(null)
    }
  }

  const handleDelete = async (item) => {
    setContextMenu(null)
    try {
      if (item.type === 'folder') {
        await tauriBridge.deleteFolder(item.id)
      } else {
        // Bei Dateien: Vault-Eintrag per manifest_block_id löschen
        // Nutzt den existierenden deleteEntryFromStore → MsgEntryDelete-Pfad
        await tauriBridge.deleteEntry(item.manifest_block_id || item.id)
      }
      await loadFolder(currentFolder)
    } catch (err) {
      setError(err.message)
    }
  }

  const handleUploadSuccess = async () => {
    setShowUpload(false)
    await loadFolder(currentFolder)
  }

  const handleContextMenu = (e, item) => {
    e.preventDefault()
    e.stopPropagation()
    setContextMenu({ x: e.clientX, y: e.clientY, item })
  }

  // Rechtsklick auf leeren Bereich (Background, nicht auf Datei/Ordner-Karte)
  const handleAreaContextMenu = (e) => {
    if (e.target.closest('[data-file-item]')) return // item has its own menu
    e.preventDefault()
    setContextMenu({ x: e.clientX, y: e.clientY, item: null }) // null = background
  }

  const handleOpenFile = (file) => {
    setContextMenu(null)
    setViewingFile({
      id: file.manifest_block_id,
      title: file.file_name,
      fields: {
        file_name:         file.file_name,
        mime_type:         file.mime_type,
        total_size:        String(file.total_size),
        manifest_block_id: file.manifest_block_id,
      },
    })
  }

  const formatSize = (bytes) => {
    if (!bytes || bytes === 0) return ''
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  }

  const fileIcon = (mimeType = '') => {
    if (mimeType.startsWith('image/')) return '🖼'
    if (mimeType === 'application/pdf') return '📕'
    if (mimeType.includes('word') || mimeType.includes('docx')) return '📝'
    if (mimeType.includes('zip') || mimeType.includes('tar')) return '📦'
    return '📄'
  }

  const totalItems = contents.folders.length + contents.files.length

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="flex items-center gap-2 px-4 py-3 border-b border-border bg-surface-base">
        {/* Breadcrumbs */}
        <nav className="flex items-center gap-1 flex-1 min-w-0 text-sm">
          <button
            onClick={() => navigateTo('', 'Root')}
            className="text-text-secondary hover:text-text-primary transition-colors font-medium"
          >
            FileVault
          </button>
          {breadcrumbs.map((bc, i) => (
            <span key={bc.id} className="flex items-center gap-1">
              <span className="text-text-disabled">/</span>
              <button
                onClick={() => navigateTo(bc.id, bc.name)}
                className={`${i === breadcrumbs.length - 1 ? 'text-text-primary font-medium' : 'text-text-secondary hover:text-text-primary'} transition-colors truncate max-w-[120px]`}
              >
                {bc.name}
              </button>
            </span>
          ))}
        </nav>

        <div className="flex items-center gap-1.5 shrink-0">
          <button
            onClick={() => loadFolder(currentFolder)}
            title="Aktualisieren"
            className="h-7 w-7 flex items-center justify-center rounded text-text-tertiary hover:text-text-primary hover:bg-surface-subtle transition-fast text-xs"
          >
            ↻
          </button>
          <button
            onClick={() => { setNewFolderMode(true); setContextMenu(null) }}
            className="h-7 px-2.5 rounded text-xs font-medium text-text-secondary border border-border hover:bg-surface-subtle transition-fast"
          >
            + Ordner
          </button>
          <button
            onClick={() => { setShowUpload(true); setContextMenu(null) }}
            className="h-7 px-3 rounded text-xs font-semibold text-white bg-accent hover:bg-accent-hover transition-fast"
          >
            ↑ Hochladen
          </button>
        </div>
      </div>

      {/* Error banner */}
      {error && (
        <div className="px-4 py-2 bg-danger/10 border-b border-danger/20 text-xs text-danger flex items-center justify-between">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="text-danger/60 hover:text-danger">✕</button>
        </div>
      )}

      {/* Main content */}
      <div className="flex-1 overflow-y-auto p-4" onContextMenu={handleAreaContextMenu}>
        {loading ? (
          <div className="flex items-center justify-center py-16 text-text-tertiary text-sm gap-2">
            <div className="w-4 h-4 border-2 border-accent border-t-transparent rounded-full animate-spin" />
            Laden…
          </div>
        ) : (
          <>
            {/* New folder inline input */}
            {newFolderMode && (
              <div className="flex items-center gap-2 mb-3 p-2 rounded-lg border border-accent/40 bg-accent/5">
                <span className="text-lg">📁</span>
                <input
                  ref={newFolderInputRef}
                  type="text"
                  value={newFolderName}
                  onChange={e => setNewFolderName(e.target.value)}
                  onKeyDown={e => {
                    if (e.key === 'Enter') handleCreateFolder()
                    if (e.key === 'Escape') { setNewFolderMode(false); setNewFolderName('') }
                  }}
                  placeholder="Ordnername…"
                  className="flex-1 h-7 px-2 text-sm bg-transparent border-none outline-none text-text-primary placeholder:text-text-disabled"
                />
                <button onClick={handleCreateFolder} className="h-7 px-2.5 rounded text-xs font-semibold bg-accent text-white">OK</button>
                <button onClick={() => { setNewFolderMode(false); setNewFolderName('') }} className="h-7 px-2 text-text-tertiary hover:text-text-primary text-xs">✕</button>
              </div>
            )}

            {/* Empty state */}
            {totalItems === 0 && !newFolderMode && (
              <div className="flex flex-col items-center justify-center py-16 text-center text-text-tertiary">
                <div className="text-4xl mb-3">📂</div>
                <p className="text-sm font-medium text-text-secondary">Dieser Ordner ist leer</p>
                <p className="text-xs mt-1">Erstelle einen Ordner oder lade eine Datei hoch</p>
              </div>
            )}

            {/* Grid of items */}
            {totalItems > 0 && (
              <div className="grid gap-2" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))' }}>

                {/* Folders first */}
                {contents.folders.map(folder => (
                  <div
                    key={folder.id}
                    data-file-item="true"
                    onDoubleClick={() => navigateTo(folder.id, folder.name)}
                    onContextMenu={e => handleContextMenu(e, { ...folder, type: 'folder' })}
                    className="group relative flex flex-col items-center gap-2 p-4 rounded-xl border border-border bg-surface-base hover:bg-surface-subtle hover:border-border-strong cursor-pointer transition-fast select-none"
                  >
                    {renaming?.id === folder.id ? (
                      <input
                        ref={renameInputRef}
                        type="text"
                        value={renaming.name}
                        onChange={e => setRenaming(prev => ({ ...prev, name: e.target.value }))}
                        onKeyDown={e => {
                          if (e.key === 'Enter') handleRenameCommit()
                          if (e.key === 'Escape') setRenaming(null)
                        }}
                        onBlur={handleRenameCommit}
                        className="w-full text-center text-xs bg-surface-app border border-accent/40 rounded px-1 py-0.5 outline-none"
                        onClick={e => e.stopPropagation()}
                      />
                    ) : (
                      <>
                        <span className="text-3xl">📁</span>
                        <span className="text-xs font-medium text-text-primary text-center leading-tight truncate w-full text-center">{folder.name}</span>
                      </>
                    )}
                  </div>
                ))}

                {/* Files */}
                {contents.files.map(file => (
                  <div
                    key={file.id}
                    data-file-item="true"
                    onDoubleClick={() => handleOpenFile(file)}
                    onContextMenu={e => handleContextMenu(e, { ...file, type: 'file' })}
                    className="group relative flex flex-col items-center gap-2 p-4 rounded-xl border border-border bg-surface-base hover:bg-surface-subtle hover:border-border-strong cursor-pointer transition-fast select-none"
                  >
                    <span className="text-3xl">{fileIcon(file.mime_type)}</span>
                    <span className="text-xs font-medium text-text-primary text-center leading-tight truncate w-full text-center" title={file.file_name}>{file.file_name}</span>
                    {file.total_size > 0 && (
                      <span className="text-[10px] text-text-tertiary">{formatSize(file.total_size)}</span>
                    )}
                  </div>
                ))}
              </div>
            )}
          </>
        )}
      </div>

      {/* Context Menu */}
      <AnimatePresence>
        {contextMenu && (
          <motion.div
            data-context-menu="true"
            key="ctx"
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            transition={{ duration: 0.1 }}
            style={{ position: 'fixed', top: contextMenu.y, left: contextMenu.x, zIndex: 9999 }}
            className="bg-surface-base border border-border rounded-lg shadow-xl py-1 min-w-[160px]"
          >
            {/* Background context menu (right-click on empty area) */}
            {contextMenu.item === null && (
              <>
                <button
                  onClick={() => { setNewFolderMode(true); setContextMenu(null) }}
                  className="w-full px-4 py-2 text-left text-sm text-text-primary hover:bg-surface-subtle transition-fast"
                >
                  📁 Neuen Ordner erstellen
                </button>
                <button
                  onClick={() => { setShowUpload(true); setContextMenu(null) }}
                  className="w-full px-4 py-2 text-left text-sm text-text-primary hover:bg-surface-subtle transition-fast"
                >
                  ↑ Datei hochladen
                </button>
              </>
            )}

            {/* Item context menus */}
            {contextMenu.item?.type === 'file' && (
              <button
                onClick={() => handleOpenFile(contextMenu.item)}
                className="w-full px-4 py-2 text-left text-sm text-text-primary hover:bg-surface-subtle transition-fast"
              >
                Öffnen
              </button>
            )}
            {contextMenu.item?.type === 'folder' && (
              <button
                onClick={() => navigateTo(contextMenu.item.id, contextMenu.item.name)}
                className="w-full px-4 py-2 text-left text-sm text-text-primary hover:bg-surface-subtle transition-fast"
              >
                Öffnen
              </button>
            )}
            {contextMenu.item?.type === 'folder' && (
              <button
                onClick={() => {
                  setRenaming({ id: contextMenu.item.id, name: contextMenu.item.name, type: 'folder' })
                  setContextMenu(null)
                }}
                className="w-full px-4 py-2 text-left text-sm text-text-primary hover:bg-surface-subtle transition-fast"
              >
                Umbenennen
              </button>
            )}
            {contextMenu.item !== null && (
              <button
                onClick={() => handleDelete(contextMenu.item)}
                className="w-full px-4 py-2 text-left text-sm text-danger hover:bg-danger/5 transition-fast"
              >
                Löschen
              </button>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {/* Upload modal */}
      <AnimatePresence>
        {showUpload && (
          <motion.div
            key="upload-modal"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
            onClick={() => setShowUpload(false)}
          >
            <motion.div
              initial={{ scale: 0.95, y: 8 }}
              animate={{ scale: 1, y: 0 }}
              exit={{ scale: 0.95, y: 8 }}
              className="bg-surface-base border border-border rounded-xl shadow-2xl p-6 w-full max-w-md"
              onClick={e => e.stopPropagation()}
            >
              <h3 className="text-sm font-semibold text-text-primary mb-4">
                Datei hochladen
                {breadcrumbs.length > 0 && (
                  <span className="ml-2 text-text-tertiary font-normal">
                    → {breadcrumbs[breadcrumbs.length - 1]?.name}
                  </span>
                )}
              </h3>
              <FileVaultUpload
                folderId={currentFolder}
                onSuccess={handleUploadSuccess}
                onCancel={() => setShowUpload(false)}
              />
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* File viewer */}
      {viewingFile && (
        <FileVaultViewer
          entry={viewingFile}
          isOpen={true}
          onClose={() => setViewingFile(null)}
          onSave={async (newManifestBlockId, fileName, mimeType) => {
            // After saving an edited file, reload the folder
            setViewingFile(null)
            await loadFolder(currentFolder)
          }}
        />
      )}
    </div>
  )
}
