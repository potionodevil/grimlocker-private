import { useEffect, useRef } from 'react'
import { useGrimStore } from '../store/useGrimStore'

/**
 * Intercepts the Tauri window close event ONLY when closeBehavior = 'minimize'.
 * For 'quit' (default) we never touch the close event — Tauri's native path runs.
 * When minimizing: hides the window; the system tray provides Show / Quit actions.
 */
export function useWindowClose() {
  const closeBehavior = useGrimStore((s) => s.preferences.closeBehavior)
  const unlistenRef = useRef(null)

  useEffect(() => {
    // For 'quit' mode: do nothing — let Tauri close the window naturally.
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
