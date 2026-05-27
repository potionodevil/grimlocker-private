import { useCallback, useEffect, useRef, useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { WorkspaceSwitcher } from '../workspace/WorkspaceSwitcher'

export function Topbar({ onSearchOpen, onAddEntry }) {
  const { daemonStatus } = useGrimStore()

  const handleWorkspaceSwitch = () => {
    // When workspace switches, the daemon will dispatch AUTH.LOGOUT
    // which will redirect to login screen in the main app flow
  }

  // Cmd+K / Ctrl+K shortcut
  useEffect(() => {
    const handler = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        onSearchOpen?.()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onSearchOpen])

  return (
    <header className="h-16 shrink-0 flex items-center px-6 gap-4 bg-surface-base border-b border-border">
      {/* Search trigger */}
      <button
        onClick={onSearchOpen}
        className="flex-1 max-w-lg flex items-center gap-2 h-9 px-3 rounded-md bg-surface-subtle border border-border text-text-tertiary text-sm hover:border-border-strong transition-fast"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
        </svg>
        <span className="flex-1 text-left">Search vault entries, SSH keys, certs…</span>
        <kbd className="hidden sm:inline-flex items-center gap-0.5 px-1.5 h-5 rounded text-xs bg-surface-base border border-border text-text-tertiary font-sans">
          ⌘K
        </kbd>
      </button>

      <div className="ml-auto flex items-center gap-3">
        <button
          onClick={onAddEntry}
          className="h-8 px-3 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover transition-fast flex items-center gap-1.5"
        >
          <span className="text-base leading-none">+</span>
          <span className="hidden sm:inline">Add</span>
        </button>

        <WorkspaceSwitcher onWorkspaceSwitch={handleWorkspaceSwitch} />

        {/* Daemon status dot */}
        <span
          className="w-2 h-2 rounded-full"
          style={{ backgroundColor: daemonStatus === 'online' ? 'var(--success)' : 'var(--text-disabled)' }}
          title={`Daemon ${daemonStatus}`}
        />

        {/* User avatar */}
        <div className="w-7 h-7 rounded-full bg-accent-subtle flex items-center justify-center text-accent text-sm font-semibold select-none">
          J
        </div>
      </div>
    </header>
  )
}
