import { useCallback } from 'react'
import { useGrimStore } from '../store/useGrimStore'

/**
 * Returns a copy function that writes text to the clipboard
 * and schedules an auto-clear after clipboardClearSeconds (0 = never).
 */
export function useCopyToClipboard() {
  const clearSeconds = useGrimStore((s) => s.preferences.clipboardClearSeconds ?? 30)

  const copy = useCallback(async (text) => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      // Fallback for older WebView environments
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
          // Only clear if the clipboard still contains our value
          const current = await navigator.clipboard.readText().catch(() => null)
          if (current === text) {
            await navigator.clipboard.writeText('')
          }
        } catch {
          // readText may be denied — just attempt a clear
          navigator.clipboard.writeText('').catch(() => {})
        }
      }, clearSeconds * 1000)
    }
  }, [clearSeconds])

  return copy
}
