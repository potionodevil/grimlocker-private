import { useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'
import { FileVaultUpload } from './FileVaultUpload'

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
    fields: [], // file upload UI, no manual fields
  },
]

export function AddEntryModal({ open, onClose }) {
  const [type, setType]             = useState('password')
  const [saving, setSaving]         = useState(false)
  const [generating, setGenerating] = useState(false) // SSH key generation
  const [form, setForm]             = useState({})
  const fetchEntries                = useGrimStore((s) => s.fetchEntries)

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
   * Generate an Ed25519 SSH key pair via the TOOL.SSH_GEN kernel event.
   * On success, auto-fills publicKey + privateKey fields. The key pair is
   * also automatically saved to the vault by the daemon (save_to_vault: true).
   */
  const handleGenerateSSHKey = async () => {
    const comment = form.title?.trim() || form.username?.trim() || 'grimlocker-generated'
    setGenerating(true)
    try {
      const result = await tauriBridge.generateSSHKey(comment, true)
      // The private key is stored in the vault by the daemon; we surface the
      // public key in the textarea so the user can copy it immediately.
      setForm((prev) => ({
        ...prev,
        publicKey:  result.public_key  || '',
        privateKey: `(stored securely in vault — entry ID: ${result.entry_id || 'unknown'})`,
        // Hint the user about the fingerprint
        notes: `Fingerprint: ${result.fingerprint || ''}`,
      }))
      if (result.entry_id) {
        // Key is already saved — refresh entries and close modal.
        await fetchEntries()
        alert(`SSH key pair generated!\n\nPublic key:\n${result.public_key}\n\nFingerprint: ${result.fingerprint}`)
        onClose()
        setForm({})
        setType('password')
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
                      onClick={() => { setType(t.id); setForm({}) }}
                      className={[
                        'px-3 h-7 rounded-md text-sm transition-fast',
                        type === t.id
                          ? 'bg-accent-subtle text-accent font-medium'
                          : 'text-text-secondary hover:text-text-primary hover:bg-surface-subtle',
                      ].join(' ')}
                    >
                      {t.id === 'file_vault' ? '📁 ' : ''}{t.label}
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
                    {/* SSH Generate button — shown above the SSH fields */}
                    {type === 'ssh' && (
                      <div className="flex items-center justify-between py-2 px-3 bg-surface-subtle rounded-lg">
                        <div>
                          <p className="text-sm font-medium text-text-primary">Ed25519 Key Pair</p>
                          <p className="text-xs text-text-secondary mt-0.5">
                            Auto-generate a secure key pair and save it directly to the vault.
                          </p>
                        </div>
                        <button
                          onClick={handleGenerateSSHKey}
                          disabled={generating}
                          className="ml-4 shrink-0 h-8 px-3 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-fast flex items-center gap-1.5"
                        >
                          {generating ? (
                            <>
                              <span className="inline-block w-3 h-3 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                              Generating…
                            </>
                          ) : (
                            <>⚡ Generate</>
                          )}
                        </button>
                      </div>
                    )}

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
              </div>

              {/* Footer — hidden for file_vault (FileVaultUpload has its own buttons) */}
              {!isFileVault && (
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
