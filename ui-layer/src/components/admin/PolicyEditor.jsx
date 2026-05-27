import { useState } from 'react'
import { Button } from '../ui/Button'
import { Select } from '../ui/Select'
import { Toggle } from '../ui/Toggle'

export function PolicyEditor() {
  const [subject,  setSubject]  = useState('john')
  const [action,   setAction]   = useState('storage.read')
  const [resource, setResource] = useState('entry:*')
  const [effect,   setEffect]   = useState('allow')
  const [timeWindow,  setTimeWindow]  = useState(false)
  const [mfaRequired, setMfaRequired] = useState(true)
  const [ipAllowlist, setIpAllowlist] = useState(false)
  const [saved, setSaved] = useState(false)

  const handleSave = () => {
    setSaved(true)
    setTimeout(() => setSaved(false), 2000)
  }

  return (
    <div>
      <h2 className="text-lg font-semibold text-text-primary mb-3">Policy Rules</h2>
      <div className="bg-surface-base border border-border rounded-md shadow-xs p-5 space-y-5">

        {/* Subject / Action / Resource / Effect */}
        <div className="grid grid-cols-2 gap-4">
          <Select label="Subject" value={subject} onChange={(e) => setSubject(e.target.value)}>
            <option value="john">john</option>
            <option value="admin">admin</option>
            <option value="*">* (everyone)</option>
          </Select>

          <Select label="Action" value={action} onChange={(e) => setAction(e.target.value)}>
            <option value="storage.read">storage.read</option>
            <option value="storage.write">storage.write</option>
            <option value="storage.delete">storage.delete</option>
            <option value="vault.unlock">vault.unlock</option>
          </Select>

          <Select label="Resource" value={resource} onChange={(e) => setResource(e.target.value)}>
            <option value="entry:*">entry:* (all entries)</option>
            <option value="/vault">/ vault</option>
            <option value="entry:passwords">entry:passwords</option>
          </Select>

          <div>
            <p className="text-sm text-text-secondary font-medium mb-1">Effect</p>
            <div className="flex items-center gap-4 h-8">
              <label className="flex items-center gap-1.5 cursor-pointer">
                <input
                  type="radio" name="effect" value="allow" checked={effect === 'allow'}
                  onChange={() => setEffect('allow')}
                  className="accent-[var(--accent)]"
                />
                <span className="text-sm text-text-primary">Allow</span>
              </label>
              <label className="flex items-center gap-1.5 cursor-pointer">
                <input
                  type="radio" name="effect" value="deny" checked={effect === 'deny'}
                  onChange={() => setEffect('deny')}
                  className="accent-[var(--danger)]"
                />
                <span className="text-sm text-text-primary">Deny</span>
              </label>
            </div>
          </div>
        </div>

        {/* Divider */}
        <div className="divider" />

        {/* Conditions */}
        <div>
          <p className="text-sm font-medium text-text-secondary mb-3">Conditions</p>
          <div className="space-y-3">
            <Toggle checked={timeWindow}  onChange={setTimeWindow}  label="Time Window (09:00 → 18:00)" />
            <Toggle checked={mfaRequired} onChange={setMfaRequired} label="MFA Required" />
            <Toggle checked={ipAllowlist} onChange={setIpAllowlist} label="IP Allowlist" />
          </div>
        </div>

        {/* Actions */}
        <div className="flex items-center justify-end gap-3 pt-1">
          <Button variant="ghost" size="sm">Cancel</Button>
          <Button variant="primary" size="sm" onClick={handleSave}>
            {saved ? '✓ Saved' : 'Save Policy'}
          </Button>
        </div>
      </div>
    </div>
  )
}
