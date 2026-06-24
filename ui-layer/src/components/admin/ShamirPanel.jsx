import { useState } from 'react'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * ShamirPanel — UI für Shamir Secret Sharing des Backup-Keys.
 * Zwei Modi:
 *   Split  — gibt ein Secret in N Shares auf (z.B. an 5 Personen verteilen, 3-of-5 benötigt)
 *   Combine — stellt das Secret aus ≥K Shares wieder her
 */
export function ShamirPanel() {
  const [mode, setMode] = useState('split') // 'split' | 'combine'

  return (
    <div className="p-6 space-y-6 max-w-2xl">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Schlüsselteilung (Shamir)</h1>
        <p className="text-xs text-text-tertiary mt-0.5">
          Teile einen geheimen Schlüssel in N Shares auf — mindestens K werden zum Wiederherstellen benötigt.
          Geeignet für Enterprise-Backup-Szenarien, wo kein einzelner Mitarbeiter das Backup allein einspielen kann.
        </p>
      </div>

      {/* Mode toggle */}
      <div className="flex gap-1 p-1 bg-surface-subtle rounded-xl w-fit">
        {['split', 'combine'].map((m) => (
          <button
            key={m}
            onClick={() => setMode(m)}
            className={`px-4 py-1.5 rounded-lg text-xs font-medium transition-fast ${
              mode === m
                ? 'bg-surface-base text-text-primary shadow-sm'
                : 'text-text-tertiary hover:text-text-secondary'
            }`}
          >
            {m === 'split' ? 'Aufteilen' : 'Zusammenführen'}
          </button>
        ))}
      </div>

      {mode === 'split' ? <SplitForm /> : <CombineForm />}
    </div>
  )
}

function SplitForm() {
  const [secret, setSecret]   = useState('')
  const [n, setN]             = useState(5)
  const [k, setK]             = useState(3)
  const [shares, setShares]   = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError]     = useState(null)
  const [copied, setCopied]   = useState(null)

  const handleSplit = async () => {
    setLoading(true)
    setError(null)
    setShares(null)
    try {
      const secretHex = toHex(secret.trim())
      const result = await tauriBridge.shamirSplit(secretHex, n, k)
      setShares(result.shares)
    } catch (e) {
      setError(e?.message ?? 'Fehler beim Aufteilen.')
    } finally {
      setLoading(false)
    }
  }

  const copyShare = (sh) => {
    const text = `x=${sh.x} y=${sh.y_hex}`
    navigator.clipboard.writeText(text).then(() => {
      setCopied(sh.x)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  return (
    <div className="space-y-4">
      {/* Secret input */}
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-text-secondary">Geheimnis (UTF-8 Text oder Hex)</label>
        <textarea
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          rows={3}
          placeholder="z.B. einen Backup-Schlüssel oder ein Passwort..."
          className="w-full px-3 py-2 rounded-lg bg-surface-subtle border border-border text-sm text-text-primary placeholder:text-text-tertiary resize-none focus:outline-none focus:ring-1 focus:ring-accent font-mono"
        />
      </div>

      {/* N / K */}
      <div className="flex gap-4">
        <NumberField label="Shares gesamt (N)" value={n} min={2} max={20} onChange={setN} />
        <NumberField label="Mindestanzahl (K)" value={k} min={2} max={n} onChange={(v) => setK(Math.min(v, n))} />
      </div>

      <p className="text-xs text-text-tertiary">
        Das Geheimnis wird in <strong className="text-text-secondary">{n}</strong> Shares aufgeteilt.
        Mindestens <strong className="text-text-secondary">{k}</strong> davon werden zur Wiederherstellung benötigt.
      </p>

      {error && (
        <div className="px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">{error}</div>
      )}

      <button
        onClick={handleSplit}
        disabled={loading || !secret.trim()}
        className="h-9 px-5 rounded-lg text-sm font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center gap-2"
      >
        {loading && <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
        {loading ? 'Teile auf…' : 'Aufteilen'}
      </button>

      {shares && (
        <div className="space-y-2">
          <p className="text-xs font-medium text-text-secondary">
            {shares.length} Shares erzeugt — je an eine Vertrauensperson weitergeben:
          </p>
          <div className="space-y-2">
            {shares.map((sh) => (
              <div key={sh.x} className="flex items-center gap-2 bg-surface-subtle rounded-lg px-3 py-2.5">
                <span className="text-xs font-mono font-bold text-accent w-8 shrink-0">S{sh.x}</span>
                <span className="text-xs font-mono text-text-secondary truncate flex-1">{sh.y_hex}</span>
                <button
                  onClick={() => copyShare(sh)}
                  className="shrink-0 text-[10px] font-medium px-2 py-1 rounded-md bg-surface-base border border-border text-text-tertiary hover:text-text-primary transition-fast"
                >
                  {copied === sh.x ? 'Kopiert!' : 'Kopieren'}
                </button>
              </div>
            ))}
          </div>
          <div className="px-3 py-2 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs">
            Jeder Share muss sicher aufbewahrt werden. Das Original-Geheimnis sollte nach Verteilung vernichtet werden.
          </div>
        </div>
      )}
    </div>
  )
}

function CombineForm() {
  const [rows, setRows]       = useState([{ x: '', y: '' }, { x: '', y: '' }, { x: '', y: '' }])
  const [result, setResult]   = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError]     = useState(null)
  const [copied, setCopied]   = useState(false)

  const updateRow = (i, field, val) => {
    setRows((r) => r.map((row, idx) => idx === i ? { ...row, [field]: val } : row))
  }

  const addRow = () => setRows((r) => [...r, { x: '', y: '' }])
  const removeRow = (i) => setRows((r) => r.filter((_, idx) => idx !== i))

  const handleCombine = async () => {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const shares = rows
        .filter((r) => r.x && r.y)
        .map((r) => ({ x: parseInt(r.x, 10), y_hex: r.y.trim() }))

      if (shares.length < 2) {
        setError('Mindestens 2 gültige Shares eingeben.')
        return
      }

      const res = await tauriBridge.shamirCombine(shares)
      setResult(fromHex(res.secret_hex))
    } catch (e) {
      setError(e?.message ?? 'Fehler beim Zusammenführen.')
    } finally {
      setLoading(false)
    }
  }

  const copyResult = () => {
    if (!result) return
    navigator.clipboard.writeText(result).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <label className="text-xs font-medium text-text-secondary">Shares eingeben</label>
          <button onClick={addRow} className="text-xs text-accent hover:underline">+ Share hinzufügen</button>
        </div>
        {rows.map((row, i) => (
          <div key={i} className="flex items-center gap-2">
            <input
              value={row.x}
              onChange={(e) => updateRow(i, 'x', e.target.value)}
              placeholder="Nr."
              className="w-14 px-2 py-2 rounded-lg bg-surface-subtle border border-border text-sm font-mono text-center focus:outline-none focus:ring-1 focus:ring-accent"
            />
            <input
              value={row.y}
              onChange={(e) => updateRow(i, 'y', e.target.value)}
              placeholder="Share-Wert (Hex)"
              className="flex-1 px-3 py-2 rounded-lg bg-surface-subtle border border-border text-xs font-mono text-text-primary placeholder:text-text-tertiary focus:outline-none focus:ring-1 focus:ring-accent"
            />
            {rows.length > 2 && (
              <button onClick={() => removeRow(i)} className="text-text-tertiary hover:text-danger transition-fast">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                  <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
                </svg>
              </button>
            )}
          </div>
        ))}
      </div>

      {error && (
        <div className="px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">{error}</div>
      )}

      <button
        onClick={handleCombine}
        disabled={loading}
        className="h-9 px-5 rounded-lg text-sm font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center gap-2"
      >
        {loading && <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
        {loading ? 'Stelle her…' : 'Geheimnis wiederherstellen'}
      </button>

      {result && (
        <div className="space-y-2">
          <p className="text-xs font-medium text-text-secondary">Wiederhergestelltes Geheimnis:</p>
          <div className="flex items-start gap-2 bg-surface-subtle rounded-lg px-3 py-3">
            <span className="text-sm font-mono text-text-primary flex-1 break-all">{result}</span>
            <button
              onClick={copyResult}
              className="shrink-0 text-[10px] font-medium px-2 py-1 rounded-md bg-surface-base border border-border text-text-tertiary hover:text-text-primary transition-fast"
            >
              {copied ? 'Kopiert!' : 'Kopieren'}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function NumberField({ label, value, min, max, onChange }) {
  return (
    <div className="space-y-1.5 flex-1">
      <label className="text-xs font-medium text-text-secondary">{label}</label>
      <div className="flex items-center gap-1">
        <button
          onClick={() => onChange(Math.max(min, value - 1))}
          className="w-8 h-9 flex items-center justify-center rounded-lg bg-surface-subtle border border-border text-text-secondary hover:bg-border transition-fast"
        >−</button>
        <span className="flex-1 h-9 flex items-center justify-center text-sm font-mono font-semibold text-text-primary bg-surface-subtle border border-border rounded-lg">
          {value}
        </span>
        <button
          onClick={() => onChange(Math.min(max, value + 1))}
          className="w-8 h-9 flex items-center justify-center rounded-lg bg-surface-subtle border border-border text-text-secondary hover:bg-border transition-fast"
        >+</button>
      </div>
    </div>
  )
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function toHex(str) {
  if (/^[0-9a-fA-F]+$/.test(str) && str.length % 2 === 0) return str
  return Array.from(new TextEncoder().encode(str)).map((b) => b.toString(16).padStart(2, '0')).join('')
}

function fromHex(hex) {
  try {
    const bytes = new Uint8Array(hex.match(/.{2}/g).map((b) => parseInt(b, 16)))
    return new TextDecoder().decode(bytes)
  } catch { return hex }
}
