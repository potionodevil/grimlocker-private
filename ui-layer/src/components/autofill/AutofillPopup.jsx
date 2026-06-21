import { useEffect, useState, useRef, useCallback } from 'react'
import { invoke } from '@tauri-apps/api/core'

const IS_TAURI = Boolean(window.__TAURI_INTERNALS__)

// Loggt sowohl ins Terminal (via Rust) als auch in die Browser-Konsole
function log(msg) {
  console.log('[GL:Popup]', msg)
  if (IS_TAURI) {
    invoke('log_autofill', { message: msg }).catch(() => {})
  }
}

const FILL_MODE = { FULL: 'full', PASS: 'pass' }

const DEMO_ENTRIES = [
  { id: 'demo-1', title: 'GitLab', fields: { username: 'user@example.com', url: 'gitlab.com' } },
  { id: 'demo-2', title: 'GitHub', fields: { username: 'user@example.com', url: 'github.com' } },
  { id: 'demo-3', title: 'Google', fields: { username: 'user@gmail.com',   url: 'google.com' } },
]

function parseEntriesFromUrl() {
  try {
    const params = new URLSearchParams(window.location.search)
    const raw = params.get('entries')
    if (!raw) return null
    return JSON.parse(decodeURIComponent(raw))
  } catch (_) {
    return null
  }
}

export function AutofillPopup() {
  const urlEntries = parseEntriesFromUrl()

  useEffect(() => {
    log(`mounted — IS_TAURI=${IS_TAURI}, urlEntries=${urlEntries?.length ?? 'null'}`)
  }, [])

  const [allEntries] = useState(IS_TAURI ? (urlEntries || []) : DEMO_ENTRIES)
  const [filtered, setFiltered]   = useState(allEntries)
  const [activeIdx, setActiveIdx] = useState(0)
  const [query, setQuery]         = useState('')
  const [fillMode, setFillMode]   = useState(FILL_MODE.FULL)
  const searchRef = useRef(null)

  useEffect(() => {
    log(`entries loaded: ${allEntries.length}`)
    setTimeout(() => searchRef.current?.focus(), 50)
  }, [])

  const confirmSelection = useCallback(async (idx, mode) => {
    const resolvedIdx  = idx ?? activeIdx
    const resolvedMode = mode ?? fillMode
    const entry = filtered[resolvedIdx]
    log(`confirmSelection: idx=${resolvedIdx} mode=${resolvedMode} entry=${entry?.id ?? 'NONE'}`)
    if (!entry?.id) { log('ERROR: no entry at index'); return }
    if (IS_TAURI) {
      try {
        log(`invoking confirm_autofill entryId=${entry.id}`)
        await invoke('confirm_autofill', { entryId: entry.id, fillMode: resolvedMode })
        log('confirm_autofill invoke done')
      } catch (err) {
        log(`confirm_autofill FAILED: ${err}`)
        console.error('[GL:Popup] confirm_autofill error:', err)
      }
    } else {
      log(`dev mode: would select "${entry.title}" mode=${resolvedMode}`)
    }
  }, [filtered, activeIdx, fillMode])

  const cancel = useCallback(async () => {
    log('cancel called')
    if (IS_TAURI) {
      try {
        await invoke('cancel_autofill')
      } catch (err) {
        log(`cancel_autofill FAILED: ${err}`)
      }
    }
  }, [])

  const applyFilter = useCallback((q) => {
    setQuery(q)
    const lower = q.toLowerCase()
    setFiltered(lower
      ? allEntries.filter(e =>
          (e.title || '').toLowerCase().includes(lower) ||
          (e.fields?.username || '').toLowerCase().includes(lower) ||
          (e.fields?.url || '').toLowerCase().includes(lower))
      : [...allEntries])
    setActiveIdx(0)
  }, [allEntries])

  useEffect(() => {
    const onKey = (e) => {
      switch (e.key) {
        case 'ArrowDown':  e.preventDefault(); setActiveIdx(i => Math.min(i + 1, filtered.length - 1)); break
        case 'ArrowUp':    e.preventDefault(); setActiveIdx(i => Math.max(i - 1, 0)); break
        case 'Enter':      e.preventDefault(); confirmSelection(activeIdx, fillMode); break
        case 'Escape':     e.preventDefault(); cancel(); break
        case 'Tab':
          if (!e.shiftKey) {
            e.preventDefault()
            setFillMode(m => m === FILL_MODE.FULL ? FILL_MODE.PASS : FILL_MODE.FULL)
          }
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [filtered.length, activeIdx, fillMode, confirmSelection, cancel])

  const listRef = useRef(null)
  useEffect(() => {
    listRef.current?.querySelector('.af-active')?.scrollIntoView({ block: 'nearest' })
  }, [activeIdx])

  const modeLabel = fillMode === FILL_MODE.FULL ? 'User + Passwort' : 'Nur Passwort'
  const modeColor = fillMode === FILL_MODE.FULL ? '#0abde3' : '#a29bfe'

  return (
    <div style={{
      display: 'flex', flexDirection: 'column', height: '100vh',
      background: '#0a0a0f', color: '#c8d6e5',
      fontFamily: "'Inter', -apple-system, sans-serif",
      fontSize: '13px', userSelect: 'none', overflow: 'hidden',
    }}>
      <div style={{ padding: '8px 10px 4px', borderBottom: '1px solid #1e272e', flexShrink: 0 }}>
        <input
          ref={searchRef}
          type="text" value={query}
          onChange={e => applyFilter(e.target.value)}
          placeholder="Filtern…"
          autoComplete="off" spellCheck={false}
          style={{
            width: '100%', background: '#111318', border: '1px solid #2d3436',
            borderRadius: '6px', color: '#dfe6e9', fontSize: '12px',
            padding: '5px 10px', outline: 'none', boxSizing: 'border-box',
          }}
        />
      </div>

      <div style={{
        display: 'flex', alignItems: 'center', gap: '6px',
        padding: '4px 12px', borderBottom: '1px solid #1e272e', flexShrink: 0,
        fontSize: '10px', color: '#57606f',
      }}>
        <span>Eintragen:</span>
        <button onClick={() => setFillMode(FILL_MODE.FULL)} style={{
          padding: '2px 8px', borderRadius: '4px', border: 'none', cursor: 'pointer',
          fontSize: '10px', fontFamily: 'inherit',
          background: fillMode === FILL_MODE.FULL ? '#0abde3' : '#2d3436',
          color: fillMode === FILL_MODE.FULL ? '#0a0a0f' : '#8395a7',
        }}>User + Passwort</button>
        <button onClick={() => setFillMode(FILL_MODE.PASS)} style={{
          padding: '2px 8px', borderRadius: '4px', border: 'none', cursor: 'pointer',
          fontSize: '10px', fontFamily: 'inherit',
          background: fillMode === FILL_MODE.PASS ? '#a29bfe' : '#2d3436',
          color: fillMode === FILL_MODE.PASS ? '#0a0a0f' : '#8395a7',
        }}>Nur Passwort</button>
        <span style={{ marginLeft: 'auto', color: '#636e72', fontSize: '9px' }}>Tab zum Wechseln</span>
      </div>

      <div style={{
        padding: '4px 12px 2px', fontSize: '10px', textTransform: 'uppercase',
        letterSpacing: '1px', color: '#57606f', flexShrink: 0,
      }}>
        Passwort wählen ({filtered.length})
      </div>

      <div ref={listRef} style={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
        {filtered.length === 0 ? (
          <div style={{ padding: '20px', textAlign: 'center', color: '#636e72', fontSize: '12px' }}>
            Keine Einträge gefunden.
          </div>
        ) : filtered.map((entry, i) => {
          const active = i === activeIdx
          const letter = (entry.title || '?')[0].toUpperCase()
          const sub = entry.fields?.username || entry.username || entry.fields?.url || entry.url || '–'
          return (
            <div key={entry.id}
              className={active ? 'af-active' : ''}
              onClick={() => confirmSelection(i, fillMode)}
              onMouseEnter={() => setActiveIdx(i)}
              style={{
                display: 'flex', alignItems: 'center', gap: '8px',
                padding: '7px 12px', cursor: 'pointer',
                background: active ? '#1e272e' : 'transparent',
              }}
            >
              <div style={{
                width: 28, height: 28, borderRadius: 6, flexShrink: 0,
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                background: active ? modeColor : '#2d3436',
                color: active ? '#0a0a0f' : modeColor,
                fontSize: 14, fontWeight: 700,
              }}>{letter}</div>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{ fontSize: 12, fontWeight: 600, color: '#dfe6e9', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{entry.title || 'Unbekannt'}</div>
                <div style={{ fontSize: 10, color: '#636e72', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>{sub}</div>
              </div>
              {active && (
                <div style={{ fontSize: 9, color: modeColor, flexShrink: 0, textAlign: 'right' }}>↵ {modeLabel}</div>
              )}
            </div>
          )
        })}
      </div>

      <div style={{
        padding: '4px 12px 6px', borderTop: '1px solid #1e272e', flexShrink: 0,
        display: 'flex', justifyContent: 'space-between', fontSize: 9, color: '#57606f',
      }}>
        <Kbd label="↑↓" ch="Navigieren" />
        <Kbd label="↵" ch="Eintragen" />
        <Kbd label="Tab" ch="Modus" />
        <Kbd label="Esc" ch="Abbrechen" />
      </div>
    </div>
  )
}

function Kbd({ label, ch }) {
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
      <span style={{
        background: '#2d3436', border: '1px solid #636e72', borderRadius: 3,
        padding: '1px 4px', fontSize: 9, color: '#b2bec3',
      }}>{label}</span> {ch}
    </span>
  )
}
