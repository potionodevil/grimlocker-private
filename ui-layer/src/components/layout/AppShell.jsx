import { useState, useEffect, useCallback, useRef } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import { Sidebar } from './Sidebar'
import { Topbar } from './Topbar'
import { DetailPanel } from './DetailPanel'
import { VaultGrid } from '../vault/VaultGrid'
import { SearchBar } from '../vault/SearchBar'
import { AddEntryModal } from '../vault/AddEntryModal'
import { AuditLog } from '../admin/AuditLog'
import { PolicyEditor } from '../admin/PolicyEditor'
import { HealthCards } from '../admin/HealthCards'
import { SyncPanel } from '../admin/SyncPanel'
import { PreferencesPanel } from '../preferences/PreferencesPanel'
import { DebugPanel } from '../debug/DebugPanel'
import { FileVaultBrowser } from '../vault/FileVaultBrowser'
import { CreateGroupModal } from '../vault/CreateGroupModal'
import { isDevMode } from '../../utils/devMode'

export function AppShell() {
  const { activeEntry, clearActiveEntry, preferences } = useGrimStore()
  const [view, setView]                         = useState(preferences.startupView || 'all')
  const [searchOpen, setSearchOpen]             = useState(false)
  const [addOpen, setAddOpen]                   = useState(false)
  const [createGroupOpen, setCreateGroupOpen]   = useState(false)

  // File vault folder state — lifted up so sidebar can show folders
  const [fileVaultFolders, setFileVaultFolders] = useState([])   // root-level folders
  const [activeFileVaultFolder, setActiveFileVaultFolder] = useState('')  // '' = root

  // Password group state
  const [activePasswordGroup, setActivePasswordGroup] = useState(null)

  // Fetch root-level file vault folders when switching to FILE_VAULT
  const foldersLoadedRef = useRef(false)

  const loadFileVaultFolders = useCallback(async () => {
    try {
      const result = await tauriBridge.listFolder('')
      setFileVaultFolders(result.folders || [])
    } catch {
      // Non-critical; sidebar just won't show folders
    }
  }, [])

  useEffect(() => {
    if (view === 'FILE_VAULT') {
      loadFileVaultFolders()
      foldersLoadedRef.current = true
    }
  }, [view, loadFileVaultFolders])

  const handleNavigate = (newView) => {
    setView(newView)
    if (newView !== 'passwords') setActivePasswordGroup(null)
    if (newView !== 'FILE_VAULT') {
      setActiveFileVaultFolder('')
    }
  }

  const handleFileVaultFolder = (id, _name) => {
    setActiveFileVaultFolder(id)
  }

  const renderMain = () => {
    if (view === 'debug' && isDevMode()) return <DebugPanel />
    if (view === 'preferences') return <PreferencesPanel />
    if (view === 'audit') return (
      <div className="p-6 space-y-6">
        <HealthCards />
        <AuditLog />
      </div>
    )
    if (view === 'admin' || view === 'policy') return (
      <div className="p-6 space-y-6">
        <HealthCards />
        <PolicyEditor />
      </div>
    )
    if (view === 'health') return <div className="p-6"><HealthCards /></div>
    if (view === 'sync') return <SyncPanel />
    if (view === 'FILE_VAULT') return (
      <FileVaultBrowser
        jumpToFolder={activeFileVaultFolder}
        onFolderChange={(id) => setActiveFileVaultFolder(id)}
        onRootFoldersChange={setFileVaultFolders}
      />
    )
    return (
      <VaultGrid
        filter={view}
        group={view === 'passwords' ? activePasswordGroup : null}
      />
    )
  }

  return (
    <div className="flex h-screen overflow-hidden bg-surface-app">
      <Sidebar
        activeView={view}
        onNavigate={handleNavigate}
        fileVaultFolders={fileVaultFolders}
        activeFileVaultFolder={activeFileVaultFolder}
        onFileVaultFolder={handleFileVaultFolder}
        activePasswordGroup={activePasswordGroup}
        onPasswordGroup={setActivePasswordGroup}
        onCreatePasswordGroup={() => setCreateGroupOpen(true)}
      />

      <div className="flex flex-col flex-1 min-w-0">
        <Topbar onSearchOpen={() => setSearchOpen(true)} onAddEntry={() => setAddOpen(true)} />
        <main className="flex-1 overflow-y-auto">
          {renderMain()}
        </main>
      </div>

      <SearchBar open={searchOpen} onClose={() => setSearchOpen(false)} />
      <AddEntryModal open={addOpen} onClose={() => setAddOpen(false)} />
      <DetailPanel entry={activeEntry} onClose={clearActiveEntry} />
      {createGroupOpen && (
        <CreateGroupModal
          onClose={() => setCreateGroupOpen(false)}
          onCreated={() => setCreateGroupOpen(false)}
        />
      )}
    </div>
  )
}
