import { useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useGrimStore } from '../../store/useGrimStore'

const GROUP_COLORS = [
  '#0055FF', '#7C3AED', '#DC2626', '#D97706', '#16A34A', '#0891B2',
  '#DB2777', '#EA580C', '#65A30D', '#0284C7',
]

export function CreateGroupModal({ onClose, onCreated }) {
  const { addPasswordGroup } = useGrimStore()
  const [label, setLabel]     = useState('')
  const [color, setColor]     = useState(GROUP_COLORS[0])

  const handleCreate = () => {
    const name = label.trim()
    if (!name) return
    addPasswordGroup({
      id:    `grp_${Date.now().toString(36)}`,
      label: name,
      color,
    })
    onCreated?.()
  }

  return (
    <AnimatePresence>
      <motion.div
        key="create-group-backdrop"
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        exit={{ opacity: 0 }}
        className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm"
        onClick={onClose}
      >
        <motion.div
          initial={{ opacity: 0, scale: 0.95, y: 8 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.95, y: 8 }}
          transition={{ duration: 0.18 }}
          className="bg-surface-base border border-border rounded-xl shadow-2xl p-6 w-full max-w-sm mx-4"
          onClick={(e) => e.stopPropagation()}
        >
          <h3 className="text-base font-semibold text-text-primary mb-4">New Password Group</h3>

          <div className="space-y-4">
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">
                Group Name
              </label>
              <input
                type="text"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') handleCreate(); if (e.key === 'Escape') onClose() }}
                placeholder="e.g. Work, Social, Finance…"
                autoFocus
                className="w-full h-9 px-3 text-sm bg-surface-subtle border border-border rounded-lg text-text-primary placeholder:text-text-disabled focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent/20 transition"
              />
            </div>

            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">
                Color
              </label>
              <div className="flex items-center gap-2 flex-wrap">
                {GROUP_COLORS.map((c) => (
                  <button
                    key={c}
                    onClick={() => setColor(c)}
                    title={c}
                    className="w-6 h-6 rounded-full border-2 transition-all"
                    style={{
                      backgroundColor: c,
                      borderColor: color === c ? 'var(--text-primary)' : 'transparent',
                      transform: color === c ? 'scale(1.15)' : 'scale(1)',
                    }}
                  />
                ))}
              </div>
            </div>
          </div>

          <div className="flex items-center justify-end gap-2 mt-5">
            <button
              onClick={onClose}
              className="h-8 px-4 text-sm text-text-secondary hover:text-text-primary hover:bg-surface-subtle rounded-lg transition-fast"
            >
              Cancel
            </button>
            <button
              onClick={handleCreate}
              disabled={!label.trim()}
              className="h-8 px-4 text-sm font-medium text-white bg-accent hover:bg-accent-hover rounded-lg transition-fast disabled:opacity-40 disabled:cursor-not-allowed"
            >
              Create Group
            </button>
          </div>
        </motion.div>
      </motion.div>
    </AnimatePresence>
  )
}
