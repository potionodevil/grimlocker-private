import { useState, useEffect } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useGrimStore } from '../../store/useGrimStore'
import { useCopyToClipboard } from '../../hooks/useClipboard'

const MASK = '••••••••'

// Type badge labels (no emoji)
const TYPE_LABELS = {
  password:    'PW',
  ssh:         'SSH',
  cert:        'CERT',
  certificate: 'CERT',
  file_vault:  'FILE',
}

function MaskedField({ label, value, revealed }) {
  return (
    <div>
      <p className="text-sm text-text-tertiary mb-0.5">{label}</p>
      <p className={`text-sm break-all font-mono leading-relaxed ${revealed ? 'text-text-primary' : 'text-text-tertiary select-none'}`}>
        {revealed ? (value || '—') : (value ? MASK : '—')}
      </p>
    </div>
  )
}

/** Plain visible field — for non-sensitive data like public keys. */
function PlainField({ label, value }) {
  return (
    <div>
      <p className="text-sm text-text-tertiary mb-0.5">{label}</p>
      <p className="text-xs break-all font-mono text-text-primary leading-relaxed bg-surface-subtle rounded px-2 py-1.5 select-all">
        {value || '—'}
      </p>
    </div>
  )
}

function CopyButton({ value }) {
  const [copied, setCopied] = useState(false)
  const copy = useCopyToClipboard()

  if (!value) return null

  const handleCopy = async () => {
    await copy(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <button
      onClick={handleCopy}
      className="text-xs text-accent hover:text-accent-hover transition-fast"
    >
      {copied ? 'Copied' : 'Copy'}
    </button>
  )
}

export function DetailPanel({ entry, onClose }) {
  const { decryptEntry, lockEntry, decryptedEntries } = useGrimStore()
  const [isDecrypting, setIsDecrypting] = useState(false)

  const decrypted = entry ? decryptedEntries[entry.id] : null
  const isRevealed = decrypted != null

  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose?.() }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  if (!entry) return null

  const handleReveal = async () => {
    setIsDecrypting(true)
    try {
      await decryptEntry(entry.id)
    } catch (err) {
      console.error('[DetailPanel] Reveal failed:', err)
    } finally {
      setIsDecrypting(false)
    }
  }

  const handleLock = () => {
    lockEntry(entry.id)
  }

  const entryData = decrypted?.data || {}
  const entryType = decrypted?.type || entry?.type || entry?.category?.toLowerCase() || 'unknown'
  const typeLabel = TYPE_LABELS[entryType] || entryType.toUpperCase().slice(0, 4)

  // Public key / fingerprint are exposed in metadata (non-sensitive).
  const publicKey  = entry.publicKey  || entryData.publicKey
  const fingerprint = entry.fingerprint || entryData.fingerprint

  return (
    <AnimatePresence>
      <>
        <motion.div
          key="backdrop"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.15 }}
          className="fixed inset-0 bg-black/20 z-20"
          onClick={onClose}
        />

        <motion.aside
          key="panel"
          initial={{ x: '100%' }}
          animate={{ x: 0 }}
          exit={{ x: '100%' }}
          transition={{ duration: 0.25, ease: [0.16, 1, 0.3, 1] }}
          className="fixed right-0 top-0 bottom-0 w-96 bg-surface-base border-l border-border shadow-md z-30 flex flex-col"
        >
          {/* Header */}
          <div className="h-14 flex items-center justify-between px-5 border-b border-border shrink-0">
            <div className="flex items-center gap-2.5 min-w-0">
              <span className="shrink-0 px-1.5 py-0.5 rounded text-xs font-mono font-semibold bg-surface-subtle text-text-tertiary border border-border">
                {typeLabel}
              </span>
              <h2 className="text-base font-semibold text-text-primary truncate">
                {entry.title || 'Entry Details'}
              </h2>
            </div>
            <button
              onClick={onClose}
              className="shrink-0 w-7 h-7 flex items-center justify-center rounded-md text-text-tertiary hover:bg-surface-subtle hover:text-text-primary transition-fast ml-2"
            >
              &#x2715;
            </button>
          </div>

          {/* Body */}
          <div className="flex-1 overflow-y-auto p-5 space-y-4">
            <MaskedField label="Title" value={entry.title} revealed={true} />

            {/* ── Password entry ── */}
            {entryType === 'password' && (
              <>
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Username</p>
                    <CopyButton value={isRevealed ? entryData.username : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.username || entry.username} revealed={isRevealed} />
                </div>
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Password</p>
                    <CopyButton value={isRevealed ? entryData.password : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.password} revealed={isRevealed} />
                </div>
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">URL</p>
                    <CopyButton value={isRevealed ? entryData.url : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.url || entry.url} revealed={isRevealed} />
                </div>
                <MaskedField label="Notes" value={entryData.notes || entry.notes} revealed={isRevealed} />
              </>
            )}

            {/* ── SSH key entry ── */}
            {entryType === 'ssh' && (
              <>
                {/* Public key — always visible (not sensitive) */}
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Public Key</p>
                    <CopyButton value={publicKey} />
                  </div>
                  <PlainField label="" value={publicKey} />
                </div>

                {/* Fingerprint — always visible */}
                {fingerprint && (
                  <div>
                    <p className="text-sm text-text-tertiary mb-0.5">Fingerprint</p>
                    <p className="text-xs font-mono text-text-secondary">{fingerprint}</p>
                  </div>
                )}

                {/* Private key — only after Reveal */}
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Private Key</p>
                    <CopyButton value={isRevealed ? entryData.privateKey : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.privateKey} revealed={isRevealed} />
                </div>

                {/* Username (for manual SSH entries) */}
                {(entryData.username || entry.username) && (
                  <div>
                    <div className="flex items-center justify-between mb-0.5">
                      <p className="text-sm text-text-tertiary">Username</p>
                      <CopyButton value={isRevealed ? entryData.username : undefined} />
                    </div>
                    <MaskedField label="" value={entryData.username || entry.username} revealed={isRevealed} />
                  </div>
                )}
              </>
            )}

            {/* ── Certificate entry ── */}
            {(entryType === 'cert' || entryType === 'certificate') && (
              <>
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Domain</p>
                    <CopyButton value={isRevealed ? entryData.domain : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.domain} revealed={isRevealed} />
                </div>
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Certificate</p>
                    <CopyButton value={isRevealed ? entryData.certificate : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.certificate} revealed={isRevealed} />
                </div>
                <div>
                  <div className="flex items-center justify-between mb-0.5">
                    <p className="text-sm text-text-tertiary">Private Key</p>
                    <CopyButton value={isRevealed ? entryData.privateKey : undefined} />
                  </div>
                  <MaskedField label="" value={entryData.privateKey} revealed={isRevealed} />
                </div>
              </>
            )}

            <MaskedField
              label="Modified"
              value={entry.updatedAt ? new Date(entry.updatedAt / 1e6).toLocaleString() : '—'}
              revealed={true}
            />
          </div>

          {/* Footer */}
          <div className="shrink-0 p-4 border-t border-border flex gap-2 justify-end">
            {isRevealed ? (
              <button
                onClick={handleLock}
                className="h-9 px-4 rounded-md text-sm font-medium text-text-secondary bg-surface-subtle hover:bg-border transition-fast"
              >
                Lock Entry
              </button>
            ) : (
              <button
                onClick={handleReveal}
                disabled={isDecrypting}
                className="h-9 px-4 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-fast"
              >
                {isDecrypting ? 'Decrypting...' : 'Reveal'}
              </button>
            )}
            <button onClick={onClose}
              className="h-9 px-4 rounded-md text-sm font-medium text-text-secondary hover:bg-surface-subtle transition-fast"
            >
              Close
            </button>
          </div>
        </motion.aside>
      </>
    </AnimatePresence>
  )
}
