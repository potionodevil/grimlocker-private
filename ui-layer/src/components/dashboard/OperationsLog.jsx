import { useGrimStore } from '../../store/useGrimStore'

const OP_COLORS = {
  GET_HEADER: 'text-cyber-cyan/70',
  UPDATE_HEADER: 'text-cyber-amber/70',
  UPDATE_CIPHERTEXT: 'text-cyber-green/80',
  GENERATE_MATRIX: 'text-cyber-green',
  PROGRESS: 'text-cyber-cyan/40',
  default: 'text-cyber-cyan/50',
}

export function OperationsLog() {
  const ops = useGrimStore((s) => s.operationsLog)
  const status = useGrimStore((s) => s.daemonStatus)

  const statusColor = {
    ready: 'bg-cyber-green',
    connecting: 'bg-cyber-amber animate-pulse',
    offline: 'bg-cyber-red',
    error: 'bg-cyber-red animate-pulse',
  }[status] ?? 'bg-cyber-cyan animate-pulse'

  return (
    <div className="flex flex-col gap-2 p-3 h-full overflow-hidden">
      <div className="flex items-center gap-2">
        <div className={`w-2 h-2 rounded-full ${statusColor}`} />
        <span className="text-cyber-cyan/60 text-xs font-mono uppercase tracking-wider">
          {status === 'ready' ? 'Encrypted & Watching' : status.toUpperCase()}
        </span>
      </div>
      <div className="flex-1 overflow-hidden flex flex-col gap-0.5">
        {ops.slice(0, 12).map((op, i) => (
          <div key={i} className="flex gap-2 font-mono text-xs leading-4">
            <span className="text-cyber-cyan/20 shrink-0">
              {new Date(op.time).toLocaleTimeString('de', { hour12: false })}
            </span>
            <span className={OP_COLORS[op.type] ?? OP_COLORS.default}>{op.type}</span>
            <span className="text-cyber-cyan/30">{op.detail}</span>
          </div>
        ))}
        {ops.length === 0 && (
          <div className="text-cyber-cyan/20 text-xs font-mono">awaiting operations...</div>
        )}
      </div>
    </div>
  )
}
