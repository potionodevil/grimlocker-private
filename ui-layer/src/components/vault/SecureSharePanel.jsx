import { useState } from 'react'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * SecureSharePanel — eingebettet in Detail-Ansicht oder Modal.
 * Erlaubt das Erstellen und Einlösen von einmaligen, verschlüsselten Entry-Links.
 *
 * Props:
 *   entry — {id, title} — der aktuell ausgewählte Entry (für "Teilen"-Tab)
 */
export function SecureSharePanel({ entry }) {
  const [mode, setMode] = useState('create') // 'create' | 'redeem'

  return (
    <div className="space-y-4">
      {/* Mode toggle */}
      <div className="flex gap-1 p-1 bg-surface-subtle rounded-xl w-fit">
        {[['create', 'Teilen'], ['redeem', 'Einlösen']].map(([m, label]) => (
          <button
            key={m}
            onClick={() => setMode(m)}
            className={`px-4 py-1.5 rounded-lg text-xs font-medium transition-fast ${
              mode === m
                ? 'bg-surface-base text-text-primary shadow-sm'
                : 'text-text-tertiary hover:text-text-secondary'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {mode === 'create'
        ? <CreateShare entry={entry} />
        : <RedeemShare />}
    </div>
  )
}

function CreateShare({ entry }) {
  const [ttl, setTtl]           = useState(24)
  const [oneTime, setOneTime]   = useState(true)
  const [result, setResult]     = useState(null)
  const [loading, setLoading]   = useState(false)
  const [error, setError]       = useState(null)
  const [copied, setCopied]     = useState(false)
  const [revoked, setRevoked]   = useState(false)

  const handleCreate = async () => {
    if (!entry?.id) return
    setLoading(true)
    setError(null)
    setResult(null)
    setRevoked(false)
    try {
      const r = await tauriBridge.createShare(entry.id, ttl, oneTime)
      setResult(r)
    } catch (e) {
      setError(e?.message ?? 'Fehler beim Erstellen.')
    } finally {
      setLoading(false)
    }
  }

  const copyToken = () => {
    if (!result?.token) return
    navigator.clipboard.writeText(result.token).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  const handleRevoke = async () => {
    if (!result?.share_id) return
    try {
      await tauriBridge.revokeShare(result.share_id)
      setRevoked(true)
    } catch (e) {
      setError(e?.message ?? 'Widerrufen fehlgeschlagen.')
    }
  }

  if (!entry?.id) {
    return (
      <p className="text-xs text-text-tertiary">Bitte wähle zuerst einen Vault-Eintrag aus.</p>
    )
  }

  return (
    <div className="space-y-4">
      {/* Entry info */}
      <div className="px-3 py-2.5 rounded-lg bg-surface-subtle border border-border flex items-center gap-2.5">
        <div className="w-8 h-8 rounded-lg bg-accent/10 flex items-center justify-center text-accent">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8}>
            <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>
          </svg>
        </div>
        <div>
          <p className="text-xs font-medium text-text-primary">{entry.title}</p>
          <p className="text-[10px] text-text-tertiary">Dieser Eintrag wird geteilt</p>
        </div>
      </div>

      {/* Options */}
      <div className="space-y-3">
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-text-secondary">Gültigkeitsdauer</label>
          <div className="flex gap-1.5">
            {[1, 6, 24, 72].map((h) => (
              <button
                key={h}
                onClick={() => setTtl(h)}
                className={`flex-1 py-1.5 rounded-lg text-xs font-medium border transition-fast ${
                  ttl === h
                    ? 'bg-accent text-white border-accent'
                    : 'bg-surface-subtle border-border text-text-secondary hover:bg-border'
                }`}
              >
                {h < 24 ? `${h}h` : `${h / 24}d`}
              </button>
            ))}
          </div>
        </div>

        <label className="flex items-center gap-2.5 cursor-pointer">
          <div
            onClick={() => setOneTime((v) => !v)}
            className={`w-8 h-5 rounded-full transition-fast relative ${oneTime ? 'bg-accent' : 'bg-border'}`}
          >
            <span className={`absolute top-0.5 w-4 h-4 bg-white rounded-full shadow transition-fast ${oneTime ? 'left-3.5' : 'left-0.5'}`} />
          </div>
          <div>
            <p className="text-xs font-medium text-text-primary">Einmal-Link</p>
            <p className="text-[10px] text-text-tertiary">Link wird nach dem ersten Einlösen ungültig</p>
          </div>
        </label>
      </div>

      {error && (
        <div className="px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">{error}</div>
      )}

      <button
        onClick={handleCreate}
        disabled={loading}
        className="w-full h-9 rounded-lg text-sm font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center justify-center gap-2"
      >
        {loading && <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
        {loading ? 'Erstelle Link…' : 'Share-Link erstellen'}
      </button>

      {result && !revoked && (
        <div className="space-y-3">
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <p className="text-xs font-medium text-text-secondary">Share-Token</p>
              <p className="text-[10px] text-text-tertiary">
                Läuft ab: {new Date(result.expires_at * 1000).toLocaleString('de-DE')}
              </p>
            </div>
            <div className="flex items-center gap-2 bg-surface-subtle rounded-lg px-3 py-2.5 border border-border">
              <span className="text-xs font-mono text-text-primary flex-1 break-all">{result.token}</span>
              <button
                onClick={copyToken}
                className="shrink-0 text-[10px] font-medium px-2 py-1 rounded-md bg-surface-base border border-border text-text-tertiary hover:text-text-primary transition-fast"
              >
                {copied ? '✓' : 'Kopieren'}
              </button>
            </div>
          </div>
          {oneTime && (
            <div className="px-3 py-2 rounded-lg bg-warning/10 border border-warning/30 text-warning text-xs">
              Dieser Link kann nur einmal eingelöst werden. Danach ist er ungültig.
            </div>
          )}
          <button
            onClick={handleRevoke}
            className="text-xs text-danger hover:underline"
          >
            Link widerrufen
          </button>
        </div>
      )}

      {revoked && (
        <div className="px-3 py-2 rounded-lg bg-surface-subtle border border-border text-text-tertiary text-xs">
          Link wurde widerrufen.
        </div>
      )}
    </div>
  )
}

function RedeemShare() {
  const [token, setToken]     = useState('')
  const [result, setResult]   = useState(null)
  const [loading, setLoading] = useState(false)
  const [error, setError]     = useState(null)
  const [copied, setCopied]   = useState(false)

  const handleRedeem = async () => {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const r = await tauriBridge.redeemShare(token.trim())
      const entry = JSON.parse(r.entry_json)
      setResult(entry)
    } catch (e) {
      setError(e?.message ?? 'Einlösen fehlgeschlagen.')
    } finally {
      setLoading(false)
    }
  }

  const copyField = (val) => {
    navigator.clipboard.writeText(val).then(() => {
      setCopied(val)
      setTimeout(() => setCopied(null), 2000)
    })
  }

  return (
    <div className="space-y-4">
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-text-secondary">Share-Token eingeben</label>
        <textarea
          value={token}
          onChange={(e) => setToken(e.target.value)}
          rows={3}
          placeholder="grimshare://..."
          className="w-full px-3 py-2 rounded-lg bg-surface-subtle border border-border text-xs font-mono text-text-primary placeholder:text-text-tertiary resize-none focus:outline-none focus:ring-1 focus:ring-accent"
        />
      </div>

      {error && (
        <div className="px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-xs">{error}</div>
      )}

      <button
        onClick={handleRedeem}
        disabled={loading || !token.trim()}
        className="w-full h-9 rounded-lg text-sm font-medium bg-accent text-white hover:bg-accent/90 disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center justify-center gap-2"
      >
        {loading && <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
        {loading ? 'Entschlüssle…' : 'Einlösen'}
      </button>

      {result && (
        <div className="space-y-3">
          <div className="px-3 py-2.5 rounded-lg bg-surface-subtle border border-border">
            <p className="text-xs font-semibold text-text-primary mb-2">{result.title}</p>
            {result.fields && Object.entries(result.fields).map(([k, v]) => (
              <div key={k} className="flex items-center gap-2 py-1">
                <span className="text-[10px] text-text-tertiary w-24 shrink-0 capitalize">{k}</span>
                <span className="text-xs text-text-secondary flex-1 font-mono truncate">
                  {k === 'password' ? '•'.repeat(Math.min(v.length, 12)) : v}
                </span>
                <button
                  onClick={() => copyField(v)}
                  className="text-[10px] text-accent hover:underline shrink-0"
                >
                  {copied === v ? '✓' : 'Kopieren'}
                </button>
              </div>
            ))}
          </div>
          <div className="px-3 py-2 rounded-lg bg-green-500/10 border border-green-500/30 text-green-400 text-xs">
            Eintrag erfolgreich entschlüsselt. Dieser Link ist nun einmalig eingelöst.
          </div>
        </div>
      )}
    </div>
  )
}
