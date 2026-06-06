import { useCallback, useEffect, useRef, useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import { WorkspaceSwitcher } from '../workspace/WorkspaceSwitcher'

/**
 * Zeigt einen ausblendenden Lock-Countdown, wenn < 2 Minuten bis zum Auto-Lock übrig sind.
 * Das ist der kleine amber-farbene Timer ganz oben rechts.
 */
function AutoLockBadge() {
  const autoLockMinutes = useGrimStore((s) => s.preferences.autoLockMinutes ?? 15)
  const [secondsLeft, setSecondsLeft] = useState(null)

  useEffect(() => {
    if (autoLockMinutes === 0) return  // disabled
    const totalMs = autoLockMinutes * 60 * 1000
    let deadline = Date.now() + totalMs

    const update = () => {
      const left = Math.max(0, Math.round((deadline - Date.now()) / 1000))
      // Badge nur anzeigen, wenn ≤ 120 Sekunden übrig sind — sonst lenkt es nur ab
      setSecondsLeft(left <= 120 ? left : null)
    }

    // Jede User-Aktivität resetet den Countdown — der Timer zählt nur bei Inaktivität runter
    const resetDeadline = () => { deadline = Date.now() + totalMs; setSecondsLeft(null) }
    const events = ['mousedown', 'keydown', 'scroll', 'touchstart']
    for (const e of events) window.addEventListener(e, resetDeadline, { passive: true })

    const interval = setInterval(update, 1000)
    return () => {
      clearInterval(interval)
      for (const e of events) window.removeEventListener(e, resetDeadline)
    }
  }, [autoLockMinutes])

  if (secondsLeft == null) return null

  const mins = Math.floor(secondsLeft / 60)
  const secs = secondsLeft % 60
  const label = mins > 0 ? `${mins}m ${secs}s` : `${secs}s`

  return (
    <div className="flex items-center gap-1.5 text-xs text-amber-500 bg-amber-500/10 border border-amber-500/30 rounded-md px-2 h-7 tabular-nums">
      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round">
        <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
        <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
      </svg>
      {label}
    </div>
  )
}

export function Topbar({ onSearchOpen, onAddEntry }) {
  const { daemonStatus, setWorkspaces, setActiveWorkspace } = useGrimStore()

  const handleWorkspaceSwitch = useCallback(async (switchedWorkspace) => {
    try {
      const workspaces = await tauriBridge.listWorkspaces()
      setWorkspaces(workspaces)
      const active = switchedWorkspace
        ? workspaces.find(ws => ws.id === switchedWorkspace.id)
        : workspaces.find(ws => ws.is_default) || workspaces[0]
      if (active) setActiveWorkspace(active)
    } catch (err) {
      console.warn('[Topbar] Failed to refresh workspaces:', err.message)
    }
  }, [setWorkspaces, setActiveWorkspace])

  // Cmd+K / Ctrl+K — der Klassiker, um die Suche zu öffnen
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
        <AutoLockBadge />
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
