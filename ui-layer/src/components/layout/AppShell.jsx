import { useState, useEffect, useCallback, useRef } from 'react'
import { clsx } from 'clsx'
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
import { BackupPanel } from '../admin/BackupPanel'
import { HealthDashboard } from '../admin/HealthDashboard'
import { ImportPanel } from '../admin/ImportPanel'
import { GeneratorPanel } from '../vault/GeneratorPanel'
import { ShamirPanel } from '../admin/ShamirPanel'
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

  // FileVault-Ordner-Status — hier hochgezogen, damit die Sidebar die Ordner sehen kann
  const [fileVaultFolders, setFileVaultFolders] = useState([])   // Root-Ordner
  const [activeFileVaultFolder, setActiveFileVaultFolder] = useState('')  // '' = Root

  // Password-Gruppen-Status — welcher Gruppen-Filter ist aktiv
  const [activePasswordGroup, setActivePasswordGroup] = useState(null)

  // Root-Ordner laden, sobald der User auf FILE_VAULT wechselt
  const foldersLoadedRef = useRef(false)

  const loadFileVaultFolders = useCallback(async () => {
    try {
      const result = await tauriBridge.listFolder('')
      setFileVaultFolders(result.folders || [])
    } catch {
      // Nicht kritisch — die Sidebar zeigt dann halt keine Ordner an
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
    if (view === 'backup') return <BackupPanel />
    if (view === 'generator') return <GeneratorPanel />
    if (view === 'health-dashboard') return <HealthDashboard />
    if (view === 'import') return <ImportPanel />
    if (view === 'shamir') return <ShamirPanel />
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

  const sidebarPosition = preferences.sidebarPosition ?? 'left'

  const sidebar = (
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
  )

  return (
    <div className={clsx('flex h-screen overflow-hidden bg-surface-app', sidebarPosition === 'right' && 'flex-row-reverse')}>
      {sidebar}

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
