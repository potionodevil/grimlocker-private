import { useCallback } from 'react'
import { useGrimStore } from '../store/useGrimStore'

/**
 * Gibt eine copy-Funktion zurück, die Text in die Zwischenablage schreibt
 * und nach clipboardClearSeconds automatisch löscht (0 = nie).
 * So verhindern wir, dass Passwörter ewig in der Zwischenablage rumliegen.
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
          // Nur leeren, falls die Zwischenablage immer noch UNSEREN Wert enthält
          // (sonst hat der User vielleicht was anderes kopiert — das wollen wir nicht killen).
          const current = await navigator.clipboard.readText().catch(() => null)
          if (current === text) {
            await navigator.clipboard.writeText('')
          }
        } catch {
          // readText kann verweigert werden (Permissions) — dann einfach blind clears versuchen
          navigator.clipboard.writeText('').catch(() => {})
        }
      }, clearSeconds * 1000)
    }
  }, [clearSeconds])

  return copy
}
