import { useState, useEffect, useCallback } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'

function StatusDot({ active }) {
  return (
    <span
      className={`inline-block w-2 h-2 rounded-full shrink-0 ${active ? 'bg-green-400 animate-pulse' : 'bg-text-disabled'}`}
    />
  )
}

function SyncCard({ label, value, sub, icon }) {
  return (
    <div className="bg-surface-base border border-border rounded-lg px-4 py-3 flex items-center gap-3 min-w-36">
      <span className="text-lg">{icon}</span>
      <div>
        <p className="text-xs text-text-tertiary">{label}</p>
        <p className="text-sm font-semibold text-text-primary">{value}</p>
        {sub && <p className="text-[10px] text-text-tertiary mt-0.5">{sub}</p>}
      </div>
    </div>
  )
}

export function SyncPanel() {
  const daemonStatus = useGrimStore((s) => s.daemonStatus)
  const [peers, setPeers]       = useState([])
  const [syncing, setSyncing]   = useState(false)
  const [lastSync, setLastSync] = useState(null)
  const [error, setError]       = useState(null)
  const [deviceId, setDeviceId] = useState(null)

  const isOnline = daemonStatus === 'online'

  const loadPeers = useCallback(async () => {
    setError(null)
    try {
      const result = await tauriBridge.listSyncPeers()
      setPeers(result.peers || [])
      setLastSync(result.last_sync_at ? new Date(result.last_sync_at) : null)
      setDeviceId(result.device_id || null)
    } catch (err) {
      setError('Sync-Status konnte nicht geladen werden. Prüfe die Daemon-Verbindung.')
      setPeers([])
    }
  }, [])

  useEffect(() => {
    if (isOnline) loadPeers()
  }, [isOnline, loadPeers])

  // Pollt alle 30 Sekunden nach neuen Peers, solange der Vault entsperrt ist
  useEffect(() => {
    if (!isOnline) return
    const interval = setInterval(loadPeers, 30_000)
    return () => clearInterval(interval)
  }, [isOnline, loadPeers])

  const handleSyncNow = async () => {
    setSyncing(true)
    setError(null)
    try {
      await tauriBridge.triggerSync()
      await loadPeers()
      setLastSync(new Date())
    } catch (err) {
      setError('Sync fehlgeschlagen. Bitte erneut versuchen.')
    } finally {
      setSyncing(false)
    }
  }

  const formatTime = (date) => {
    if (!date) return '—'
    const diff = Math.floor((Date.now() - date.getTime()) / 1000)
    if (diff < 60) return `${diff}s ago`
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`
    return date.toLocaleTimeString()
  }

  return (
    <div className="p-6 space-y-6 max-w-2xl">
      <div>
        <h2 className="text-xl font-semibold text-text-primary">LAN Sync</h2>
        <p className="text-sm text-text-secondary mt-1">
          Sync vault entries across devices on your local network using mDNS discovery and ChaCha20-Poly1305 encryption.
        </p>
      </div>

      {/* Status cards */}
      <div className="flex flex-wrap gap-3">
        <SyncCard
          icon="📡"
          label="Sync Status"
          value={isOnline ? 'Active' : 'Offline'}
          sub={isOnline ? 'mDNS listener running' : 'Daemon not connected'}
        />
        <SyncCard
          icon="🔗"
          label="Peers Found"
          value={peers.length}
          sub={peers.length === 1 ? '1 device' : `${peers.length} devices`}
        />
        <SyncCard
          icon="⏱"
          label="Last Sync"
          value={formatTime(lastSync)}
          sub="auto-pull every 60s"
        />
        {deviceId && (
          <SyncCard
            icon="🆔"
            label="This Device"
            value={deviceId.slice(0, 12) + '…'}
            sub="Ed25519 identity"
          />
        )}
      </div>

      {/* Error */}
      {error && (
        <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-danger/10 border border-danger/30 text-danger text-sm">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>
          {error}
        </div>
      )}

      {/* Sync now button */}
      <button
        onClick={handleSyncNow}
        disabled={!isOnline || syncing}
        className="h-9 px-5 rounded-lg text-sm font-medium text-white bg-accent hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center gap-2"
      >
        {syncing ? (
          <>
            <span className="w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
            Syncing…
          </>
        ) : (
          <>
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <polyline points="1 4 1 10 7 10"/><polyline points="23 20 23 14 17 14"/>
              <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4-4.64 4.36A9 9 0 0 1 3.51 15"/>
            </svg>
            Sync Now
          </>
        )}
      </button>

      {/* Peer list */}
      <div>
        <h3 className="text-sm font-semibold text-text-primary mb-3">Discovered Peers</h3>
        {!isOnline ? (
          <p className="text-sm text-text-tertiary py-6 text-center">Connect to daemon to see peers.</p>
        ) : peers.length === 0 ? (
          <div className="border border-dashed border-border rounded-xl py-10 flex flex-col items-center gap-2 text-center">
            <span className="text-3xl">📡</span>
            <p className="text-sm font-medium text-text-secondary">No peers discovered yet</p>
            <p className="text-xs text-text-tertiary">
              Make sure other Grimlocker devices are on the same network and unlocked.
            </p>
          </div>
        ) : (
          <div className="border border-border rounded-xl overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-surface-subtle">
                <tr>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold text-text-tertiary">Device ID</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold text-text-tertiary">Version</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold text-text-tertiary">Last Seen</th>
                  <th className="px-4 py-2.5 text-left text-xs font-semibold text-text-tertiary">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border">
                {peers.map((peer) => (
                  <tr key={peer.device_id} className="hover:bg-surface-subtle transition-fast">
                    <td className="px-4 py-2.5 font-mono text-xs text-text-primary">
                      {peer.device_id?.slice(0, 16)}…
                    </td>
                    <td className="px-4 py-2.5 text-xs text-text-secondary tabular-nums">
                      v{peer.version ?? '—'}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-text-secondary">
                      {peer.last_seen_at ? formatTime(new Date(peer.last_seen_at)) : '—'}
                    </td>
                    <td className="px-4 py-2.5">
                      <span className="flex items-center gap-1.5 text-xs text-text-secondary">
                        <StatusDot active={peer.reachable !== false} />
                        {peer.reachable !== false ? 'Reachable' : 'Unreachable'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Security-Infos — damit der User sieht, wie sicher der Sync ist */}
      <div className="bg-surface-subtle border border-border rounded-xl p-4 space-y-1.5">
        <p className="text-xs font-semibold text-text-primary">Security Properties</p>
        {[
          'All sync traffic encrypted with ChaCha20-Poly1305 (X25519 ECDH session key)',
          'Peers authenticated via Ed25519 challenge/response + 6-digit PIN pairing',
          'Monotonic version vectors prevent replay and downgrade attacks',
          'Sync only runs when vault is unlocked (session-gated)',
        ].map((line) => (
          <p key={line} className="text-xs text-text-secondary flex items-start gap-2">
            <span className="text-green-500 shrink-0 mt-0.5">✓</span>
            {line}
          </p>
        ))}
      </div>
    </div>
  )
}
