import { useState, useEffect, useRef, useCallback } from 'react'
import { useGrimStore } from '../store/useGrimStore'

/**
 * useCopyToClipboard — kopiert Text, löscht nach clipboardClearSeconds automatisch.
 * Rückwärtskompatibel — gibt wie vorher eine copy-Funktion zurück.
 */
export function useCopyToClipboard() {
  const clearSeconds = useGrimStore((s) => s.preferences.clipboardClearSeconds ?? 30)

  const copy = useCallback(async (text) => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      // Fallback für ältere WebView-Umgebungen (Tauri on Windows 10)
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }

    if (clearSeconds > 0) {
      setTimeout(async () => {
        try {
          const current = await navigator.clipboard.readText().catch(() => null)
          if (current === text) {
            await navigator.clipboard.writeText('')
          }
        } catch {
          navigator.clipboard.writeText('').catch(() => {})
        }
      }, clearSeconds * 1000)
    }
  }, [clearSeconds])

  return copy
}

/**
 * useClipboard — erweiterter Clipboard-Hook mit Countdown, isCopied-State
 * und sofortigem Clear bei Vault-Sperrung.
 *
 * @param {number} ttlMs — Auto-Clear-Timeout in Millisekunden (default 30000)
 * @returns {{ copy, countdown, isCopied, clearNow }}
 */
export function useClipboard(ttlMs = 30_000) {
  const [isCopied, setIsCopied]   = useState(false)
  const [countdown, setCountdown] = useState(0)
  const clearTimer                = useRef(null)
  const tickTimer                 = useRef(null)
  const daemonStatus              = useGrimStore((s) => s.daemonStatus)

  const clearNow = useCallback(() => {
    try { navigator.clipboard.writeText('') } catch { /* ignored */ }
    setIsCopied(false)
    setCountdown(0)
    clearTimeout(clearTimer.current)
    clearInterval(tickTimer.current)
  }, [])

  // Vault gesperrt → Clipboard sofort löschen
  useEffect(() => {
    if ((daemonStatus === 'locked' || daemonStatus === 'offline') && isCopied) {
      clearNow()
    }
  }, [daemonStatus, isCopied, clearNow])

  const copy = useCallback(async (text) => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.style.cssText = 'position:fixed;opacity:0'
      document.body.appendChild(ta)
      ta.select()
      document.execCommand('copy')
      document.body.removeChild(ta)
    }

    setIsCopied(true)
    const totalSec = Math.ceil(ttlMs / 1000)
    setCountdown(totalSec)

    clearTimeout(clearTimer.current)
    clearInterval(tickTimer.current)

    let remaining = totalSec
    tickTimer.current = setInterval(() => {
      remaining -= 1
      setCountdown(remaining)
      if (remaining <= 0) clearInterval(tickTimer.current)
    }, 1000)

    clearTimer.current = setTimeout(clearNow, ttlMs)
  }, [ttlMs, clearNow])

  useEffect(() => () => {
    clearTimeout(clearTimer.current)
    clearInterval(tickTimer.current)
  }, [])

  return { copy, countdown, isCopied, clearNow }
}

// CopyButton lives in CopyButton.jsx (JSX not allowed in .js files by Vite).
// Re-exported here so existing `import { CopyButton } from '…/useClipboard'` still works.
export { CopyButton } from '../components/vault/CopyButton'
