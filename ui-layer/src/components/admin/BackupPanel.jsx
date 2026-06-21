import { useState } from 'react'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'

function Section({ title, children }) {
  return (
    <div className="bg-surface-base border border-border rounded-xl p-5 space-y-4">
      <h2 className="text-sm font-semibold text-text-primary">{title}</h2>
      {children}
    </div>
  )
}

function Field({ label, children }) {
  return (
    <div className="space-y-1.5">
      <label className="text-xs text-text-tertiary">{label}</label>
      {children}
    </div>
  )
}

function Input({ value, onChange, placeholder, disabled }) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      className="w-full h-8 px-3 text-xs bg-surface-app border border-border rounded-lg text-text-primary placeholder:text-text-disabled outline-none focus:border-accent disabled:opacity-50 disabled:cursor-not-allowed transition-fast"
    />
  )
}

function Checkbox({ checked, onChange, label }) {
  return (
    <label className="flex items-center gap-2 cursor-pointer select-none">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="w-3.5 h-3.5 rounded accent-accent"
      />
      <span className="text-xs text-text-secondary">{label}</span>
    </label>
  )
}

function ActionButton({ onClick, disabled, loading, children }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled || loading}
      className="inline-flex items-center gap-2 px-4 h-8 rounded-lg text-xs font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast"
    >
      {loading && (
        <svg className="animate-spin w-3 h-3" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="2" strokeOpacity="0.3" />
          <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
        </svg>
      )}
      {children}
    </button>
  )
}

function ErrorBanner({ message }) {
  if (!message) return null
  return (
    <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
      {message}
    </div>
  )
}

function SuccessBanner({ message }) {
  if (!message) return null
  return (
    <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-green-500/10 border border-green-500/30 text-green-400 text-xs">
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
        <polyline points="20 6 9 17 4 12" />
      </svg>
      {message}
    </div>
  )
}

function MetaCard({ peek }) {
  const ts = peek.export_timestamp
    ? new Date(peek.export_timestamp * 1000).toLocaleString()
    : '—'
  return (
    <div className="bg-surface-app border border-border rounded-lg p-3 space-y-1.5 text-xs">
      <Row label="Exportiert am" value={ts} />
      <Row label="Version" value={peek.grimlocker_version || '—'} />
      <Row label="Einträge" value={peek.entry_count ?? '—'} />
      <Row label="Hardware-Tethering" value={peek.hardware_tethered ? 'Ja' : 'Nein'} />
      <Row label="Header-Integrität"
        value={peek.header_integrity_ok ? '✓ OK' : '✗ Mismatch'}
        valueClass={peek.header_integrity_ok ? 'text-green-400' : 'text-danger'}
      />
      {peek.hardware_tethered && (
        <Row label="Hardware-ID" value={peek.hardware_id_hex?.slice(0, 16) + '…'} mono />
      )}
    </div>
  )
}

function Row({ label, value, valueClass = 'text-text-primary', mono = false }) {
  return (
    <div className="flex justify-between gap-4">
      <span className="text-text-tertiary">{label}</span>
      <span className={`${valueClass} ${mono ? 'font-mono' : ''} text-right truncate max-w-48`}>{value}</span>
    </div>
  )
}

export function BackupPanel() {
  const daemonStatus = useGrimStore((s) => s.daemonStatus)
  const isOnline = daemonStatus === 'online'

  // ── Export state ──────────────────────────────────────────────────────────
  const [exportPath, setExportPath]       = useState('')
  const [hardwareTether, setHardwareTether] = useState(false)
  const [exportLoading, setExportLoading] = useState(false)
  const [exportResult, setExportResult]   = useState(null)
  const [exportError, setExportError]     = useState(null)

  const handleExport = async () => {
    if (!exportPath.trim()) { setExportError('Bitte Ziel-Pfad angeben.'); return }
    setExportLoading(true)
    setExportResult(null)
    setExportError(null)
    try {
      const res = await tauriBridge.exportBackup(exportPath.trim(), hardwareTether)
      setExportResult(res)
    } catch (e) {
      setExportError(e?.message || 'Export fehlgeschlagen.')
    } finally {
      setExportLoading(false)
    }
  }

  // ── Import state ──────────────────────────────────────────────────────────
  const [importPath, setImportPath]         = useState('')
  const [peekResult, setPeekResult]         = useState(null)
  const [peekLoading, setPeekLoading]       = useState(false)
  const [peekError, setPeekError]           = useState(null)
  const [mergeMode, setMergeMode]           = useState(true)
  const [authorizeLoading, setAuthorizeLoading] = useState(false)
  const [authorizeResult, setAuthorizeResult]   = useState(null)
  const [authorizeError, setAuthorizeError]     = useState(null)

  const handlePeek = async () => {
    if (!importPath.trim()) { setPeekError('Bitte Quell-Pfad angeben.'); return }
    setPeekLoading(true)
    setPeekResult(null)
    setPeekError(null)
    setAuthorizeResult(null)
    setAuthorizeError(null)
    try {
      const res = await tauriBridge.peekBackup(importPath.trim())
      setPeekResult(res)
    } catch (e) {
      setPeekError(e?.message || 'Datei konnte nicht gelesen werden.')
    } finally {
      setPeekLoading(false)
    }
  }

  const handleAuthorize = async () => {
    if (!peekResult?.session_id) return
    setAuthorizeLoading(true)
    setAuthorizeResult(null)
    setAuthorizeError(null)
    try {
      const res = await tauriBridge.authorizeBackup(peekResult.session_id, mergeMode)
      setAuthorizeResult(res)
      setPeekResult(null)
    } catch (e) {
      setAuthorizeError(e?.message || 'Import fehlgeschlagen.')
    } finally {
      setAuthorizeLoading(false)
    }
  }

  // ── Checksum state ────────────────────────────────────────────────────────
  const [checksumPath, setChecksumPath]     = useState('')
  const [checksumLoading, setChecksumLoading] = useState(false)
  const [checksumResult, setChecksumResult] = useState(null)
  const [checksumError, setChecksumError]   = useState(null)

  const handleChecksum = async () => {
    if (!checksumPath.trim()) { setChecksumError('Bitte Dateipfad angeben.'); return }
    setChecksumLoading(true)
    setChecksumResult(null)
    setChecksumError(null)
    try {
      const res = await tauriBridge.checksumBackup(checksumPath.trim())
      setChecksumResult(res)
    } catch (e) {
      setChecksumError(e?.message || 'Checksum fehlgeschlagen.')
    } finally {
      setChecksumLoading(false)
    }
  }

  return (
    <div className="p-6 space-y-4 max-w-2xl">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Air-Gap Backup</h1>
        <p className="text-xs text-text-tertiary mt-0.5">
          Exportiere deinen Vault in eine verschlüsselte .grimbak-Datei oder importiere ein bestehendes Backup — ohne Internetverbindung.
        </p>
      </div>

      {!isOnline && (
        <div className="px-3 py-2 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs">
          Daemon nicht verbunden. Bitte Grimlocker neu starten.
        </div>
      )}

      {/* ── Export ── */}
      <Section title="Export">
        <Field label="Ziel-Pfad (.grimbak)">
          <Input
            value={exportPath}
            onChange={setExportPath}
            placeholder="/home/user/vault-backup.grimbak"
            disabled={!isOnline || exportLoading}
          />
        </Field>
        <Checkbox
          checked={hardwareTether}
          onChange={setHardwareTether}
          label="Hardware-Tethering — Backup kann nur auf diesem Gerät importiert werden"
        />
        <div className="flex items-center gap-3">
          <ActionButton onClick={handleExport} disabled={!isOnline} loading={exportLoading}>
            Export starten
          </ActionButton>
        </div>
        <ErrorBanner message={exportError} />
        {exportResult && (
          <div className="bg-green-500/10 border border-green-500/30 rounded-lg p-3 space-y-1 text-xs">
            <div className="flex items-center gap-2 text-green-400 font-medium">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}><polyline points="20 6 9 17 4 12" /></svg>
              Export erfolgreich
            </div>
            <Row label="Datei" value={exportResult.path} />
            <Row label="Einträge" value={exportResult.entry_count} />
            <Row label="SHA-256" value={exportResult.sha256} mono />
          </div>
        )}
      </Section>

      {/* ── Import ── */}
      <Section title="Import">
        <Field label="Phase 1 — Backup-Datei prüfen">
          <div className="flex gap-2">
            <Input
              value={importPath}
              onChange={(v) => { setImportPath(v); setPeekResult(null) }}
              placeholder="/home/user/vault-backup.grimbak"
              disabled={!isOnline || peekLoading}
            />
            <ActionButton onClick={handlePeek} disabled={!isOnline || !importPath.trim()} loading={peekLoading}>
              Prüfen
            </ActionButton>
          </div>
        </Field>
        <ErrorBanner message={peekError} />

        {peekResult && (
          <div className="space-y-3">
            <p className="text-xs text-text-tertiary">Backup-Informationen:</p>
            <MetaCard peek={peekResult} />

            <div className="space-y-2 pt-1">
              <p className="text-xs font-medium text-text-secondary">Phase 2 — Import autorisieren</p>
              <Checkbox
                checked={mergeMode}
                onChange={setMergeMode}
                label="Zusammenführen — bestehende Einträge (gleiche ID) überspringen statt überschreiben"
              />
              <ActionButton onClick={handleAuthorize} disabled={!isOnline} loading={authorizeLoading}>
                Import autorisieren
              </ActionButton>
            </div>
            <ErrorBanner message={authorizeError} />
          </div>
        )}

        {authorizeResult && (
          <SuccessBanner
            message={`Import abgeschlossen — ${authorizeResult.imported_count} importiert, ${authorizeResult.skipped_count} übersprungen.`}
          />
        )}
      </Section>

      {/* ── Integrität prüfen ── */}
      <Section title="Integrität prüfen">
        <Field label="Dateipfad">
          <div className="flex gap-2">
            <Input
              value={checksumPath}
              onChange={setChecksumPath}
              placeholder="/home/user/vault-backup.grimbak"
              disabled={!isOnline || checksumLoading}
            />
            <ActionButton onClick={handleChecksum} disabled={!isOnline || !checksumPath.trim()} loading={checksumLoading}>
              SHA-256 prüfen
            </ActionButton>
          </div>
        </Field>
        <ErrorBanner message={checksumError} />
        {checksumResult && (
          <div className="bg-surface-app border border-border rounded-lg p-3 text-xs font-mono text-text-secondary break-all">
            {checksumResult.sha256}
          </div>
        )}
      </Section>
    </div>
  )
}
