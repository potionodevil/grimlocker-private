import { useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'
import { FileVaultUpload } from './FileVaultUpload'

// Passphrase-Modi für SSH-Key-Generierung:
// 'none'   — keine Passphrase (Key funktioniert ohne)
// 'auto'   — Daemon generiert 32-Zeichen Zufallspassphrase (wird EINMAL gezeigt)
// 'custom' — User gibt eigene Passphrase ein

const ENTRY_TYPES = [
  {
    id: 'password',
    label: 'Password',
    category: 'PASSWORD',
    fields: ['title', 'username', 'password', 'url', 'notes'],
  },
  {
    id: 'ssh',
    label: 'SSH Key',
    category: 'SSH_KEY',
    fields: ['title', 'username', 'privateKey', 'publicKey', 'notes'],
  },
  {
    id: 'cert',
    label: 'Certificate',
    category: 'CERTIFICATE',
    fields: ['title', 'domain', 'certificate', 'privateKey', 'notes'],
  },
  {
    id: 'file_vault',
    label: 'Files',
    category: 'FILE_VAULT',
    fields: [], // FileVault hat kein manuelles Formular — nur Drag&Drop-Upload
  },
]

export function AddEntryModal({ open, onClose }) {
  const [type, setType]                   = useState('password')
  const [saving, setSaving]               = useState(false)
  const [generating, setGenerating]       = useState(false)
  const [form, setForm]                   = useState({})
  const [sshMode, setSshMode]             = useState('generate') // 'generate' | 'manual'
  const [passphraseMode, setPassphrase]   = useState('none')    // 'none' | 'auto' | 'custom'
  const [customPassphrase, setCustomPass] = useState('')
  const fetchEntries                      = useGrimStore((s) => s.fetchEntries)

  if (!open) return null

  const activeType = ENTRY_TYPES.find((t) => t.id === type)
  const isFileVault = type === 'file_vault'

  const update = (field, value) => setForm((prev) => ({ ...prev, [field]: value }))

  const handleSave = async () => {
    if (!form.title?.trim()) return
    setSaving(true)

    const entry = {
      type,
      category: activeType.category,
      title:        form.title?.trim() || 'Untitled',
      username:     form.username     || '',
      password:     form.password     || '',
      url:          form.url          || '',
      notes:        form.notes        || '',
      privateKey:   form.privateKey   || '',
      publicKey:    form.publicKey    || '',
      domain:       form.domain       || '',
      certificate:  form.certificate  || '',
    }

    try {
      await tauriBridge.saveEntry(entry)
      await fetchEntries()
      onClose()
      setForm({})
      setType('password')
    } catch (err) {
      console.error('[AddEntry] Save failed:', err)
      alert('Failed to save entry: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  /**
   * Generiert ein Ed25519-SSH-Keypair via TOOL.SSH_GEN-Kernel-Event.
   * Der Comment wird aus dem sshKeyName-Feld als "name@grimlocker.sec" gebaut.
   * Der Daemon speichert das Keypair automatisch im Vault.
   */
  const handleGenerateSSHKey = async () => {
    const rawName = form.sshKeyName?.trim() || 'key'
    const comment = `${rawName}@grimlocker.sec`
    const useAutoPassphrase = passphraseMode === 'auto'
    const passphrase = passphraseMode === 'custom' ? customPassphrase : ''

    if (passphraseMode === 'custom' && !customPassphrase.trim()) {
      alert('Enter a passphrase or switch to Auto-generate or None.')
      return
    }

    setGenerating(true)
    try {
      const result = await tauriBridge.generateSSHKey(comment, true, passphrase, useAutoPassphrase)
      if (result.entry_id) {
        await fetchEntries()

        let successMsg = `SSH key pair generated!\n\nName: ${comment}\n\nPublic key:\n${result.public_key}\nFingerprint: ${result.fingerprint}`
        if (result.passphrase) {
          successMsg += `\n\nPassphrase (shown once — save it now):\n${result.passphrase}`
        } else if (passphraseMode === 'none') {
          successMsg += '\n\nNo passphrase — key is fully functional without one.'
        }

        alert(successMsg)
        onClose()
        setForm({})
        setType('password')
        setSshMode('generate')
        setPassphrase('none')
        setCustomPass('')
      }
    } catch (err) {
      console.error('[AddEntry] SSH key generation failed:', err)
      alert('Key generation failed: ' + err.message)
    } finally {
      setGenerating(false)
    }
  }

  const handleFileSuccess = async (manifest) => {
    await fetchEntries()
    onClose()
    setForm({})
    setType('password')
  }

  return (
    <AnimatePresence>
      {open && (
        <>
          {/* Backdrop */}
          <motion.div
            key="backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15 }}
            className="fixed inset-0 bg-black/40 z-40"
            onClick={onClose}
          />

          {/* Modal */}
          <motion.div
            key="modal"
            initial={{ opacity: 0, scale: 0.96, y: 20 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 20 }}
            transition={{ duration: 0.2, ease: [0.16, 1, 0.3, 1] }}
            className="fixed inset-0 flex items-center justify-center z-50 pointer-events-none"
          >
            <div
              className="bg-surface-base border border-border rounded-lg shadow-md w-full max-w-lg max-h-[85vh] overflow-y-auto pointer-events-auto flex flex-col"
              onClick={(e) => e.stopPropagation()}
            >
              {/* Header */}
              <div className="flex items-center justify-between px-5 h-14 border-b border-border shrink-0">
                <h2 className="text-base font-semibold text-text-primary">Add Entry</h2>
                <button
                  onClick={onClose}
                  className="w-7 h-7 flex items-center justify-center rounded-md text-text-tertiary hover:bg-surface-subtle hover:text-text-primary transition-fast"
                >
                  ✕
                </button>
              </div>

              {/* Type selector */}
              <div className="px-5 py-3 border-b border-border shrink-0">
                <div className="flex items-center gap-1">
                  {ENTRY_TYPES.map((t) => (
                    <button
                      key={t.id}
                      onClick={() => { setType(t.id); setForm({}); setSshMode('generate'); setPassphrase('none'); setCustomPass('') }}
                      className={[
                        'px-3 h-7 rounded-md text-sm transition-fast',
                        type === t.id
                          ? 'bg-accent-subtle text-accent font-medium'
                          : 'text-text-secondary hover:text-text-primary hover:bg-surface-subtle',
                      ].join(' ')}
                    >
                      {t.label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Body */}
              <div className="p-5 space-y-4 overflow-y-auto flex-1">
                {isFileVault ? (
                  /* ── File Vault tab: drag-and-drop upload ── */
                  <FileVaultUpload onSuccess={handleFileSuccess} onCancel={onClose} />
                ) : (
                  /* ── Regular entry fields ── */
                  <>
                    {/* SSH: Generate / Manual sub-tabs */}
                    {type === 'ssh' ? (
                      <>
                        {/* Sub-tab bar */}
                        <div className="flex items-center gap-1 p-1 bg-surface-subtle rounded-lg">
                          {[
                            { id: 'generate', label: 'Generate' },
                            { id: 'manual',   label: 'Manual' },
                          ].map((m) => (
                            <button
                              key={m.id}
                              onClick={() => setSshMode(m.id)}
                              className={[
                                'flex-1 h-7 rounded-md text-sm transition-fast',
                                sshMode === m.id
                                  ? 'bg-surface-base text-text-primary font-medium shadow-sm'
                                  : 'text-text-secondary hover:text-text-primary',
                              ].join(' ')}
                            >
                              {m.label}
                            </button>
                          ))}
                        </div>

                        {sshMode === 'generate' ? (
                          /* ── Generate mode ── */
                          <div className="space-y-3">
                            <div>
                              <label className="block text-sm text-text-secondary mb-1">
                                Key Name
                              </label>
                              <div className="flex items-center gap-2">
                                <input
                                  type="text"
                                  value={form.sshKeyName || ''}
                                  onChange={(e) => update('sshKeyName', e.target.value)}
                                  placeholder="e.g. server, deploy, homelab"
                                  className="flex-1 h-9 px-3 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast"
                                />
                                <span className="text-sm text-text-tertiary whitespace-nowrap">
                                  @grimlocker.sec
                                </span>
                              </div>
                              <p className="text-xs text-text-tertiary mt-1">
                                The key will be saved as{' '}
                                <span className="font-mono text-accent">
                                  {(form.sshKeyName?.trim() || 'key')}@grimlocker.sec
                                </span>
                              </p>
                            </div>

                            {/* Passphrase selector */}
                            <div>
                              <label className="block text-sm text-text-secondary mb-1.5">
                                Passphrase
                              </label>
                              <div className="flex items-center gap-1 p-1 bg-surface-subtle rounded-lg mb-2">
                                {[
                                  { id: 'none',   label: 'None' },
                                  { id: 'auto',   label: 'Auto-generate' },
                                  { id: 'custom', label: 'Custom' },
                                ].map((m) => (
                                  <button
                                    key={m.id}
                                    type="button"
                                    onClick={() => { setPassphrase(m.id); if (m.id !== 'custom') setCustomPass('') }}
                                    className={[
                                      'flex-1 h-7 rounded-md text-sm transition-fast',
                                      passphraseMode === m.id
                                        ? 'bg-surface-base text-text-primary font-medium shadow-sm'
                                        : 'text-text-secondary hover:text-text-primary',
                                    ].join(' ')}
                                  >
                                    {m.label}
                                  </button>
                                ))}
                              </div>
                              {passphraseMode === 'none' && (
                                <p className="text-xs text-text-tertiary">
                                  Key works without a passphrase. Vault encryption still protects the private key at rest.
                                </p>
                              )}
                              {passphraseMode === 'auto' && (
                                <p className="text-xs text-text-tertiary">
                                  A 32-character random passphrase will be shown once after generation. Save it immediately.
                                </p>
                              )}
                              {passphraseMode === 'custom' && (
                                <input
                                  type="password"
                                  value={customPassphrase}
                                  onChange={(e) => setCustomPass(e.target.value)}
                                  placeholder="Enter passphrase…"
                                  className="w-full h-9 px-3 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast"
                                />
                              )}
                            </div>

                            <button
                              onClick={handleGenerateSSHKey}
                              disabled={generating}
                              className="w-full h-9 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center justify-center gap-2"
                            >
                              {generating ? (
                                <>
                                  <span className="inline-block w-3.5 h-3.5 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                                  Generating Ed25519 key…
                                </>
                              ) : (
                                'Generate Ed25519 Key Pair'
                              )}
                            </button>

                            <p className="text-xs text-text-tertiary text-center">
                              Uses <span className="font-mono">crypto/ed25519</span> + <span className="font-mono">crypto/rand</span> — private key stored encrypted in vault
                            </p>
                          </div>
                        ) : (
                          /* ── Manual mode ── */
                          <>
                            {activeType.fields.map((field) => (
                              <div key={field}>
                                <label className="block text-sm text-text-secondary mb-1 capitalize">
                                  {field.replace(/([A-Z])/g, ' $1').trim()}
                                  {field === 'title' && <span className="text-danger ml-0.5">*</span>}
                                </label>
                                {field === 'notes' || field === 'privateKey' || field === 'publicKey' ? (
                                  <textarea
                                    value={form[field] || ''}
                                    onChange={(e) => update(field, e.target.value)}
                                    rows={field === 'notes' ? 3 : 6}
                                    className="w-full px-3 py-2 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast resize-none font-mono"
                                    placeholder={`Enter ${field.replace(/([A-Z])/g, ' $1').trim().toLowerCase()}…`}
                                  />
                                ) : (
                                  <input
                                    type="text"
                                    value={form[field] || ''}
                                    onChange={(e) => update(field, e.target.value)}
                                    className="w-full h-9 px-3 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast"
                                    placeholder={`Enter ${field.replace(/([A-Z])/g, ' $1').trim().toLowerCase()}…`}
                                  />
                                )}
                              </div>
                            ))}
                          </>
                        )}
                      </>
                    ) : (
                      /* ── Non-SSH entry types ── */
                      <>
                        {activeType.fields.map((field) => (
                          <div key={field}>
                            <label className="block text-sm text-text-secondary mb-1 capitalize">
                              {field.replace(/([A-Z])/g, ' $1').trim()}
                              {field === 'title' && <span className="text-danger ml-0.5">*</span>}
                            </label>
                            {field === 'notes' || field === 'privateKey' || field === 'publicKey' || field === 'certificate' ? (
                              <textarea
                                value={form[field] || ''}
                                onChange={(e) => update(field, e.target.value)}
                                rows={field === 'notes' ? 3 : 6}
                                className="w-full px-3 py-2 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast resize-none font-mono"
                                placeholder={`Enter ${field.replace(/([A-Z])/g, ' $1').trim().toLowerCase()}…`}
                              />
                            ) : (
                              <input
                                type={field === 'password' ? 'password' : 'text'}
                                value={form[field] || ''}
                                onChange={(e) => update(field, e.target.value)}
                                className="w-full h-9 px-3 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast"
                                placeholder={`Enter ${field.replace(/([A-Z])/g, ' $1').trim().toLowerCase()}…`}
                              />
                            )}
                          </div>
                        ))}
                      </>
                    )}
                  </>
                )}
              </div>

              {/* Footer — versteckt bei file_vault und SSH-Generate-Mode */}
              {!isFileVault && !(type === 'ssh' && sshMode === 'generate') && (
                <div className="shrink-0 px-5 py-4 border-t border-border flex justify-end gap-2">
                  <button
                    onClick={onClose}
                    className="h-9 px-4 rounded-md text-sm font-medium text-text-secondary hover:bg-surface-subtle transition-fast"
                  >
                    Cancel
                  </button>
                  <button
                    onClick={handleSave}
                    disabled={saving || !form.title?.trim()}
                    className="h-9 px-4 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-fast"
                  >
                    {saving ? 'Saving…' : 'Save Entry'}
                  </button>
                </div>
              )}
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}
