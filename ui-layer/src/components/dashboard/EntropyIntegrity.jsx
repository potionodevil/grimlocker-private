import { useGrimStore } from '../../store/useGrimStore'

export function EntropyIntegrity() {
  const { entropyInfo, header } = useGrimStore()

  const overrideColor = entropyInfo.overrideAttemptsLeft <= 1
    ? 'text-cyber-red'
    : entropyInfo.overrideAttemptsLeft <= 2
      ? 'text-cyber-amber'
      : 'text-cyber-green'

  return (
    <div className="h-full flex flex-col p-4">
      <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider mb-4">
        ENTROPY INTEGRITY
      </span>

      <div className="space-y-4 flex-1">
        <div>
          <div className="flex items-center justify-between mb-1">
            <span className="font-mono text-xs text-cyber-borderLight">FILE SIZE</span>
            <span className="font-mono text-xs text-cyber-cyan">{entropyInfo.fileSize || 1247} bytes</span>
          </div>
          <div className="w-full h-1 bg-cyber-dark rounded-full overflow-hidden">
            <div className="h-full bg-cyber-cyan/60 rounded-full" style={{ width: '72%' }} />
          </div>
        </div>

        <div>
          <div className="flex items-center justify-between mb-1">
            <span className="font-mono text-xs text-cyber-borderLight">BITS OF SECURITY</span>
            <span className="font-mono text-xs text-cyber-green">{entropyInfo.bitsOfSecurity}</span>
          </div>
          <div className="w-full h-1 bg-cyber-dark rounded-full overflow-hidden">
            <div className="h-full bg-cyber-green/60 rounded-full" style={{ width: '100%' }} />
          </div>
        </div>

        <div>
          <div className="flex items-center justify-between mb-1">
            <span className="font-mono text-xs text-cyber-borderLight">OVERRIDE LEFT</span>
            <span className={`font-mono text-xs ${overrideColor}`}>{entropyInfo.overrideAttemptsLeft}/4</span>
          </div>
          <div className="w-full h-1 bg-cyber-dark rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all duration-500 ${
                entropyInfo.overrideAttemptsLeft <= 1 ? 'bg-cyber-red/60' :
                entropyInfo.overrideAttemptsLeft <= 2 ? 'bg-cyber-amber/60' : 'bg-cyber-green/60'
              }`}
              style={{ width: `${(entropyInfo.overrideAttemptsLeft / 4) * 100}%` }}
            />
          </div>
        </div>

        <div>
          <div className="flex items-center justify-between mb-1">
            <span className="font-mono text-xs text-cyber-borderLight">FAILED ATTEMPTS</span>
            <span className={`font-mono text-xs ${header.failedAttempts >= 3 ? 'text-cyber-red' : 'text-cyber-amber'}`}>
              {header.failedAttempts}/3
            </span>
          </div>
          <div className="w-full h-1 bg-cyber-dark rounded-full overflow-hidden">
            <div
              className={`h-full rounded-full transition-all duration-500 ${
                header.failedAttempts >= 3 ? 'bg-cyber-red/60' : 'bg-cyber-amber/60'
              }`}
              style={{ width: `${(header.failedAttempts / 3) * 100}%` }}
            />
          </div>
        </div>
      </div>

      <div className="mt-4 pt-3 border-t border-cyber-border/30">
        <div className="flex items-center gap-2">
          <div className="w-2 h-2 rounded-full bg-cyber-green animate-pulse" />
          <span className="font-mono text-xs text-cyber-greenDim">INTEGRITY VERIFIED</span>
        </div>
      </div>
    </div>
  )
}
