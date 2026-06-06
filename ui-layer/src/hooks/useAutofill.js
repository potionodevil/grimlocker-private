import { useEffect, useCallback, useRef, useState } from 'react'
import { listen } from '@tauri-apps/api/event'
import { WebviewWindow } from '@tauri-apps/api/webviewWindow'
import { invoke } from '@tauri-apps/api/core'
import { useGrimStore } from '../store/useGrimStore'
import { tauriBridge } from '../services/tauriBridge'
import { extractKeywords, matchEntries } from '../utils/domainMatcher'

export function useAutofill() {
  const fetchEntries = useGrimStore(s => s.fetchEntries)
  const [showLocked, setShowLocked] = useState(false)
  const [pendingTitle, setPendingTitle] = useState(null)
  const processingRef = useRef(false)

  const fillEntry = useCallback(async (entry, fillMode = 'full') => {
    console.log('[GL:Autofill] fillEntry →', entry?.id, 'mode=', fillMode)
    try {
      const store = useGrimStore.getState()
      const fullEntry = await store.decryptEntry(entry.id)
      if (!fullEntry) throw new Error('Entschlüsselung fehlgeschlagen')

      // Translator gibt { data: { username, password, ... } } zurück — NICHT fields
      const data = fullEntry.data || fullEntry.fields || fullEntry
      const pw   = data?.password || ''
      const user = data?.username || ''
      console.log('[GL:Autofill] fillEntry: user?', Boolean(user), 'pw?', Boolean(pw), 'dataKeys=', Object.keys(data || {}))

      const textToType = (fillMode === 'pass' || !user) ? pw : `${user}\t${pw}`
      console.log('[GL:Autofill] invoking fill_text, length=', textToType.length)
      await invoke('fill_text', { text: textToType })
      console.log('[GL:Autofill] fill_text done ✓')
    } catch (err) {
      console.error('[GL:Autofill] fillEntry FAILED:', err)
    }
  }, [])

  const showPopup = useCallback(async (matchedEntries) => {
    console.log('[GL:Autofill] showPopup with', matchedEntries.length, 'entries')
    const safeEntries = matchedEntries.slice(0, 8).map(e => ({
      id: e.id,
      title: e.title || '',
      fields: {
        username: e.fields?.username || '',
        url: e.fields?.url || '',
      },
      category: e.category,
    }))

    // Einträge als URL-Parameter ans Popup übergeben — kein Event nötig
    const encoded = encodeURIComponent(JSON.stringify(safeEntries))
    console.log('[GL:Autofill] opening WebviewWindow, encoded entries length=', encoded.length)

    new WebviewWindow('autofill', {
      url: `/?popup=autofill&entries=${encoded}`,
      title: 'Grimlocker Autofill',
      width: 320,
      height: Math.min(420, 80 + safeEntries.length * 48),
      decorations: false,
      alwaysOnTop: true,
      skipTaskbar: true,
      resizable: false,
      center: true,
      focus: true,
    })
  }, [])

  const showPopupRef = useRef(showPopup)
  showPopupRef.current = showPopup
  const fillEntryRef = useRef(fillEntry)
  fillEntryRef.current = fillEntry

  // Listener für autofill:confirm — kommt vom Rust-Backend via confirm_autofill
  useEffect(() => {
    console.log('[GL:Autofill] registering autofill:confirm listener')
    const setup = async () => {
      const unlisten = await listen('autofill:confirm', (event) => {
        const { entryId, fillMode } = event.payload || {}
        console.log('[GL:Autofill] autofill:confirm received → entryId=', entryId, 'mode=', fillMode)
        if (!entryId) { console.warn('[GL:Autofill] no entryId in payload!'); return }
        const { entries } = useGrimStore.getState()
        console.log('[GL:Autofill] vault has', entries.length, 'entries')
        const entry = entries.find(e => e.id === entryId)
        if (!entry) { console.error('[GL:Autofill] entry not found:', entryId); return }
        fillEntryRef.current(entry, fillMode)
      })
      console.log('[GL:Autofill] autofill:confirm listener registered ✓')
      return unlisten
    }
    let unlisten = null
    setup().then(fn => { unlisten = fn })
    return () => { if (unlisten) unlisten() }
  }, [])

  const handleAutofill = useCallback(async (windowTitle, appName, url) => {
    const { entries: currentEntries } = useGrimStore.getState()

    if (!currentEntries || currentEntries.length === 0) {
      setPendingTitle({ windowTitle, appName, url })
      setShowLocked(true)
      return
    }

    const keywords = extractKeywords(windowTitle || '', appName || '')
    if (!keywords.length) return

    const matched = matchEntries(currentEntries, keywords)

    if (matched.length === 0) {
      const allPasswords = currentEntries.filter(e => {
        const cat = (e.category || '').toUpperCase()
        return cat === 'PASSWORD' || !cat
      })
      if (allPasswords.length > 0) {
        await showPopupRef.current(allPasswords)
      }
      return
    }

    if (matched.length === 1) {
      await fillEntryRef.current(matched[0])
      return
    }

    await showPopupRef.current(matched)
  }, [])

  // Strg+G Event vom Rust-Backend abhören
  useEffect(() => {
    let unlisten = () => {}

    const setup = async () => {
      try {
        unlisten = await listen('autofill:trigger', async (event) => {
          if (processingRef.current) { console.warn('[GL:Autofill] trigger ignored (still processing)'); return }
          processingRef.current = true
          const { windowTitle, appName, url } = event.payload || {}
          console.log('[GL:Autofill] trigger received:', windowTitle, appName)
          await handleAutofill(windowTitle, appName, url)
          processingRef.current = false
        })
        console.log('[GL:Autofill] autofill:trigger listener registered ✓')
      } catch (_err) {
        console.debug('[Autofill] Tauri-Event nicht verfügbar (dev mode), nutze lokalen Strg+G')
        const handler = (e) => {
          if (e.ctrlKey && e.key === 'g' && !processingRef.current) {
            e.preventDefault()
            processingRef.current = true
            handleAutofill('Demo — GitHub', 'Firefox').finally(() => {
              processingRef.current = false
            })
          }
        }
        window.addEventListener('keydown', handler)
        unlisten = () => window.removeEventListener('keydown', handler)
      }
    }

    setup()
    return () => { unlisten() }
  }, [handleAutofill])

  const handleUnlock = useCallback(async (password) => {
    try {
      await tauriBridge.unlockVault(password)
      setShowLocked(false)
      if (pendingTitle) {
        const { windowTitle, appName, url } = pendingTitle
        setPendingTitle(null)
        await new Promise(r => setTimeout(r, 800))
        await fetchEntries()
        await handleAutofill(windowTitle, appName, url)
      }
    } catch (err) {
      throw err
    }
  }, [pendingTitle, fetchEntries, handleAutofill])

  const handleCancelLocked = useCallback(() => {
    setShowLocked(false)
    setPendingTitle(null)
  }, [])

  return { showLocked, handleUnlock, handleCancelLocked }
}
