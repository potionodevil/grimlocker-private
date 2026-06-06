import { useState } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { tauriBridge } from '../../services/tauriBridge'
import { Badge } from '../ui/Badge'

export function HealthCards() {
  const { daemonStatus } = useGrimStore()
  const [panicConfirm, setPanicConfirm] = useState(false)
  const [panicStatus, setPanicStatus] = useState(null)

  const handlePanicWipe = async () => {
    if (!panicConfirm) {
      setPanicConfirm(true)
      return
    }
    try {
      setPanicStatus('wiping')
      await tauriBridge.panicWipe()
      setPanicStatus('done')
    } catch (err) {
      setPanicStatus('error')
    }
    setPanicConfirm(false)
  }

  return (
    <div>
      <h2 className="text-lg font-semibold text-text-primary mb-3">System Health</h2>
      <div className="flex gap-dp-gap flex-wrap">
        <HealthCard
          label="Kernel"
          value={daemonStatus === 'online' ? 'Ready' : 'Offline'}
          variant={daemonStatus === 'online' ? 'success' : 'danger'}
          icon="\u2B21"
        />
        <HealthCard
          label="Integrity"
          value="Clean"
          variant="success"
          icon="\u2713"
        />
        <HealthCard
          label="VFS"
          value="Mounted"
          variant="success"
          icon="\uD83D\uDDC3"
        />
      </div>

      <div className="mt-6 pt-4 border-t border-border">
        <h3 className="text-sm font-semibold text-red-400 mb-2">Danger Zone</h3>
        {/* Danger Zone — hier ist Vorsicht geboten */}
        <p className="text-xs text-text-tertiary mb-3">
          Panic Wipe destroys all vault files and zeros key material. This action is irreversible.
        </p>
        <button
          onClick={handlePanicWipe}
          disabled={panicStatus === 'wiping'}
          className={[
            'px-4 py-2 rounded-md text-sm font-medium transition-fast',
            panicConfirm
              ? 'bg-red-600 text-white hover:bg-red-700'
              : 'bg-red-500/10 text-red-400 hover:bg-red-500/20 border border-red-500/30',
            panicStatus === 'wiping' ? 'opacity-50 cursor-not-allowed' : '',
          ].join(' ')}
        >
          {panicStatus === 'wiping' ? 'Wiping...' : panicConfirm ? 'CONFIRM: Destroy Everything' : 'Panic Wipe'}
        </button>
      </div>
    </div>
  )
}

function HealthCard({ label, value, variant, icon }) {
  return (
    <div className="bg-surface-base border border-border rounded-md px-4 py-3 flex items-center gap-3 min-w-40 shadow-xs">
      <span className="text-lg">{icon}</span>
      <div>
        <p className="text-sm text-text-tertiary">{label}</p>
        <Badge variant={variant}>{value}</Badge>
      </div>
    </div>
  )
}
