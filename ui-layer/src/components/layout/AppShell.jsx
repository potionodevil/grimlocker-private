import { useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { Sidebar } from './Sidebar'
import { Topbar } from './Topbar'
import { DetailPanel } from './DetailPanel'
import { VaultGrid } from '../vault/VaultGrid'
import { SearchBar } from '../vault/SearchBar'
import { AddEntryModal } from '../vault/AddEntryModal'
import { AuditLog } from '../admin/AuditLog'
import { PolicyEditor } from '../admin/PolicyEditor'
import { HealthCards } from '../admin/HealthCards'
import { PreferencesPanel } from '../preferences/PreferencesPanel'
import { DebugPanel } from '../debug/DebugPanel'
import { isDevMode } from '../../utils/devMode'

const ADMIN_VIEWS = ['audit', 'policy', 'health']

export function AppShell() {
  const { activeEntry, clearActiveEntry } = useGrimStore()
  const [view, setView]               = useState('all')
  const [searchOpen, setSearchOpen]   = useState(false)
  const [addOpen, setAddOpen]         = useState(false)

  const renderMain = () => {
    if (view === 'debug' && isDevMode()) return <DebugPanel />
    if (view === 'preferences') return <PreferencesPanel />
    if (view === 'audit')  return (
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
    return <VaultGrid filter={view} />
  }

  return (
    <div className="flex h-screen overflow-hidden bg-surface-app">
      <Sidebar activeView={view} onNavigate={setView} />

      <div className="flex flex-col flex-1 min-w-0">
        <Topbar onSearchOpen={() => setSearchOpen(true)} onAddEntry={() => setAddOpen(true)} />
        <main className="flex-1 overflow-y-auto">
          {renderMain()}
        </main>
      </div>

      <SearchBar open={searchOpen} onClose={() => setSearchOpen(false)} />
      <AddEntryModal open={addOpen} onClose={() => setAddOpen(false)} />
      <DetailPanel entry={activeEntry} onClose={clearActiveEntry} />
    </div>
  )
}
