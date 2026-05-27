import { useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { tauriBridge } from '../../services/tauriBridge'
import { useGrimStore } from '../../store/useGrimStore'

const ENTRY_TYPES = [
  { id: 'password', label: 'Password', fields: ['title', 'username', 'password', 'url', 'notes'] },
  { id: 'ssh',      label: 'SSH Key',  fields: ['title', 'username', 'privateKey', 'publicKey', 'notes'] },
  { id: 'cert',     label: 'Certificate', fields: ['title', 'domain', 'certificate', 'privateKey', 'notes'] },
]

export function AddEntryModal({ open, onClose }) {
  const [type, setType]     = useState('password')
  const [saving, setSaving] = useState(false)
  const [form, setForm]     = useState({})
  const fetchEntries = useGrimStore((s) => s.fetchEntries)

  if (!open) return null

  const activeType = ENTRY_TYPES.find((t) => t.id === type)

  const update = (field, value) => {
    setForm((prev) => ({ ...prev, [field]: value }))
  }

  const handleSave = async () => {
    if (!form.title?.trim()) return
    setSaving(true)

    const entry = {
      type,
      title: form.title?.trim() || 'Untitled',
      username: form.username || '',
      password: form.password || '',
      url: form.url || '',
      notes: form.notes || '',
      privateKey: form.privateKey || '',
      publicKey: form.publicKey || '',
      domain: form.domain || '',
      certificate: form.certificate || '',
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
                      {t.label}
                    </button>
                  ))}
                </div>
              </div>

              {/* Form */}
              <div className="p-5 space-y-4 overflow-y-auto">
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
                        className="w-full px-3 py-2 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast resize-none"
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

              {/* Footer */}
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
            </div>
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}
