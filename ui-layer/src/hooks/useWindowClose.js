import { useEffect, useRef } from 'react'
import { useGrimStore } from '../store/useGrimStore'

/**
 * Fängt das Tauri-Window-Close-Event NUR ab, wenn closeBehavior = 'minimize'.
 * Bei 'quit' (default) lassen wir das Close-Event in Ruhe — Tauris nativer Pfad läuft.
 * Bei Minimize: Versteckt das Fenster; der System Tray bietet Show/Quit-Aktionen.
 */
export function useWindowClose() {
  const closeBehavior = useGrimStore((s) => s.preferences.closeBehavior)
  const unlistenRef = useRef(null)

  useEffect(() => {
    // Bei 'quit'-Modus: nichts tun — Tauri schliesst das Fenster ganz normal.
    if (closeBehavior !== 'minimize') {
      unlistenRef.current?.()
      unlistenRef.current = null
      return
    }

    if (typeof window.__TAURI__ === 'undefined') return

    const setup = async () => {
      try {
        const { getCurrentWindow } = await import('@tauri-apps/api/window')
        const win = getCurrentWindow()
        unlistenRef.current = await win.onCloseRequested(async (event) => {
          event.preventDefault()
          await win.hide()
        })
      } catch (err) {
        console.warn('[useWindowClose] Failed to register close handler:', err)
      }
    }

    setup()

    return () => {
      unlistenRef.current?.()
      unlistenRef.current = null
    }
  }, [closeBehavior])
}
