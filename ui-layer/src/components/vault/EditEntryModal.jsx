import { useState, useEffect } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'

const TYPE_FIELDS = {
  password: ['title', 'username', 'password', 'url', 'notes'],
  ssh: ['title', 'username', 'privateKey', 'publicKey', 'notes'],
  cert: ['title', 'domain', 'certificate', 'privateKey', 'notes'],
  certificate: ['title', 'domain', 'certificate', 'privateKey', 'notes'],
  file_vault: ['title'],
}

export function EditEntryModal({ entry, open, onClose, onSaved }) {
  const [form, setForm] = useState({})
  const [saving, setSaving] = useState(false)
  const decryptEntry = useGrimStore((s) => s.decryptEntry)
  const decryptedEntries = useGrimStore((s) => s.decryptedEntries)

  const entryType = entry?.type || entry?.category?.toLowerCase() || 'password'
  const fields = TYPE_FIELDS[entryType] || TYPE_FIELDS.password
  const isFileVault = entryType === 'file_vault'

  useEffect(() => {
    if (!entry || !open) return
    const decrypted = decryptedEntries[entry.id]
    if (decrypted?.data) {
      setForm({
        title: decrypted.data.title || entry.title || '',
        username: decrypted.data.username || '',
        password: decrypted.data.password || '',
        url: decrypted.data.url || '',
        notes: decrypted.data.notes || '',
        privateKey: decrypted.data.privateKey || '',
        publicKey: decrypted.data.publicKey || '',
        domain: decrypted.data.domain || '',
        certificate: decrypted.data.certificate || '',
      })
    } else {
      setForm({
        title: entry.title || entry.name || '',
        username: entry.username || '',
        url: entry.url || '',
        notes: entry.notes || '',
        privateKey: entry.privateKey || '',
        publicKey: entry.publicKey || '',
        domain: entry.domain || '',
        certificate: entry.certificate || '',
      })
    }
  }, [entry, open, decryptedEntries])

  if (!open || !entry) return null

  const update = (field, value) => setForm((prev) => ({ ...prev, [field]: value }))

  const handleSave = async () => {
    if (!form.title?.trim()) return
    setSaving(true)
    try {
      const category = entryType === 'ssh' ? 'SSH_KEY'
        : entryType === 'cert' || entryType === 'certificate' ? 'CERTIFICATE'
        : entryType === 'file_vault' ? 'FILE_VAULT'
        : 'PASSWORD'
      await tauriBridge.updateEntry(entry.id, {
        type: entryType,
        category,
        title: form.title?.trim() || 'Untitled',
        fields: Object.fromEntries(
          fields.filter(f => f !== 'title').map(f => [f, form[f] || ''])
        ),
      })
      onSaved?.()
    } catch (err) {
      console.error('[EditEntry] Save failed:', err)
      alert('Failed to update entry: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <AnimatePresence>
      {open && (
        <>
          <motion.div
            key="backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.15 }}
            className="fixed inset-0 bg-black/40 z-40"
            onClick={onClose}
          />
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
              <div className="flex items-center justify-between px-5 h-14 border-b border-border shrink-0">
                <h2 className="text-base font-semibold text-text-primary">Edit Entry</h2>
                <button
                  onClick={onClose}
                  className="w-7 h-7 flex items-center justify-center rounded-md text-text-tertiary hover:bg-surface-subtle hover:text-text-primary transition-fast"
                >
                  ✕
                </button>
              </div>

              <div className="p-5 space-y-4 overflow-y-auto flex-1">
                {isFileVault ? (
                  <p className="text-sm text-text-secondary">File entries can only be renamed. Other fields are managed automatically.</p>
                ) : null}

                {fields.map((field) => (
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
              </div>

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
                  {saving ? 'Saving…' : 'Save Changes'}
                </button>
              </div>
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}