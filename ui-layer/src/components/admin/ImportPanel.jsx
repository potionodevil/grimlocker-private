import { useState, useRef } from 'react'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'

const FORMATS = [
  { id: 'auto',      label: 'Automatisch erkennen' },
  { id: '1password', label: '1Password' },
  { id: 'bitwarden', label: 'Bitwarden' },
  { id: 'chrome',    label: 'Chrome / Edge' },
  { id: 'keepass',   label: 'KeePass' },
  { id: 'generic',   label: 'Generisch (title, username, password…)' },
]

export function ImportPanel() {
  const daemonStatus    = useGrimStore((s) => s.daemonStatus)
  const fetchEntries    = useGrimStore((s) => s.fetchEntries)
  const [csv, setCsv]   = useState('')
  const [format, setFmt]= useState('auto')
  const [loading, setL] = useState(false)
  const [result, setRes]= useState(null)
  const [error, setErr] = useState(null)
  const fileRef         = useRef(null)

  const isOnline = daemonStatus === 'online'

  const handleFile = (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    const reader = new FileReader()
    reader.onload = (ev) => setCsv(ev.target.result ?? '')
    reader.readAsText(file, 'utf-8')
  }

  const handleImport = async () => {
    if (!csv.trim()) { setErr('Bitte zuerst eine CSV-Datei wählen oder einfügen.'); return }
    setL(true); setErr(null); setRes(null)
    try {
      const r = await tauriBridge.importCSV(csv, format)
      if (r.error) throw new Error(r.error)
      setRes(r)
      await fetchEntries()
    } catch (e) {
      setErr(e?.message ?? 'Import fehlgeschlagen.')
    } finally {
      setL(false)
    }
  }

  return (
    <div className="p-6 space-y-5 max-w-2xl">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Passwörter importieren</h1>
        <p className="text-xs text-text-tertiary mt-0.5">
          Importiere Passwörter aus 1Password, Bitwarden, Chrome, KeePass und anderen Managern via CSV.
        </p>
      </div>

      {!isOnline && (
        <div className="px-3 py-2 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs">
          Daemon nicht verbunden.
        </div>
      )}

      {/* Format picker */}
      <div className="space-y-1.5">
        <label className="text-xs text-text-tertiary">Format</label>
        <select
          value={format}
          onChange={(e) => setFmt(e.target.value)}
          disabled={!isOnline}
          className="w-full h-8 px-3 text-xs bg-surface-base border border-border rounded-lg text-text-primary focus:outline-none focus:border-accent disabled:opacity-50"
        >
          {FORMATS.map((f) => <option key={f.id} value={f.id}>{f.label}</option>)}
        </select>
      </div>

      {/* File picker */}
      <div className="space-y-1.5">
        <label className="text-xs text-text-tertiary">CSV-Datei wählen</label>
        <div className="flex gap-2 items-center">
          <input ref={fileRef} type="file" accept=".csv,text/csv" onChange={handleFile} className="hidden" />
          <button
            onClick={() => fileRef.current?.click()}
            disabled={!isOnline}
            className="h-8 px-4 rounded-lg border border-border text-xs text-text-secondary hover:text-text-primary hover:bg-surface-subtle transition-fast disabled:opacity-40"
          >
            Datei wählen…
          </button>
          {csv && <span className="text-xs text-green-400">✓ {csv.split('\n').length - 1} Zeilen geladen</span>}
        </div>
      </div>

      {/* Paste area */}
      <div className="space-y-1.5">
        <label className="text-xs text-text-tertiary">… oder CSV-Inhalt einfügen</label>
        <textarea
          value={csv}
          onChange={(e) => setCsv(e.target.value)}
          rows={6}
          disabled={!isOnline}
          placeholder={'name,url,username,password\nGitHub,https://github.com,user@example.com,secret123'}
          className="w-full px-3 py-2 rounded-md bg-surface-base border border-border text-text-primary text-xs placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast resize-none font-mono disabled:opacity-50"
        />
      </div>

      {/* Import button */}
      <button
        onClick={handleImport}
        disabled={!isOnline || loading || !csv.trim()}
        className="h-9 px-5 rounded-lg bg-accent text-white text-sm font-medium hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center gap-2"
      >
        {loading ? (
          <><span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />Importiere…</>
        ) : 'Import starten'}
      </button>

      {/* Error */}
      {error && (
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">
          <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/>
          </svg>
          {error}
        </div>
      )}

      {/* Result */}
      {result && (
        <div className="bg-surface-base border border-border rounded-xl p-4 space-y-3">
          <div className="flex items-center gap-2 text-green-400 text-sm font-medium">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}><polyline points="20 6 9 17 4 12" /></svg>
            Import abgeschlossen
          </div>
          <div className="grid grid-cols-2 gap-3 text-xs">
            <Stat label="Importiert" value={result.imported} green />
            <Stat label="Übersprungen" value={result.skipped} />
          </div>
          {result.errors?.length > 0 && (
            <div className="space-y-1">
              <p className="text-xs text-text-tertiary font-medium">Fehler:</p>
              {result.errors.map((e, i) => (
                <p key={i} className="text-xs text-danger">{e}</p>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Format help */}
      <div className="bg-surface-subtle border border-border rounded-xl p-4 space-y-2">
        <p className="text-xs font-semibold text-text-primary">Export-Anleitung</p>
        {[
          { app: '1Password', path: 'Datei → Exportieren → 1PIF oder CSV' },
          { app: 'Bitwarden', path: 'Werkzeuge → Exportieren → .csv' },
          { app: 'Chrome',    path: 'chrome://settings/passwords → Exportieren' },
          { app: 'KeePass',   path: 'Datei → Exportieren → CSV' },
        ].map((r) => (
          <p key={r.app} className="text-xs text-text-secondary flex gap-2">
            <span className="font-medium text-text-primary w-20 shrink-0">{r.app}</span>
            {r.path}
          </p>
        ))}
      </div>
    </div>
  )
}

function Stat({ label, value, green }) {
  return (
    <div className="bg-surface-app border border-border rounded-lg px-3 py-2">
      <p className="text-text-tertiary">{label}</p>
      <p className={`text-xl font-bold mt-0.5 ${green ? 'text-green-400' : 'text-text-primary'}`}>{value}</p>
    </div>
  )
}
