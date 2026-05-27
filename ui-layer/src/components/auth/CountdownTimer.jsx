import { useCountdown } from '../../hooks/useCountdown'
import { useGrimStore } from '../../store/useGrimStore'

export function CountdownTimer() {
  const { header, isCritical } = useGrimStore()
  const { formatted, isExpired } = useCountdown(header.lockdownTimestamp)

  const borderColor = isExpired
    ? 'border-cyber-red animate-pulse-glow-red'
    : isCritical
      ? 'border-cyber-amber animate-pulse-glow-amber'
      : 'border-cyber-amber/50'

  const textColor = isExpired ? 'text-cyber-red' : 'text-cyber-amber'
  const labelColor = isExpired ? 'text-cyber-red/70' : 'text-cyber-amber/70'

  if (header.failedAttempts < 3) return null

  return (
    <div className={`rounded-sm border ${borderColor} bg-cyber-dark/80 px-8 py-6`}>
      <div className="text-center">
        <p className={`font-mono text-xs uppercase tracking-widest ${labelColor} mb-2`}>
          {isExpired ? 'LOCKDOWN EXPIRED' : 'LOCKDOWN TIMER'}
        </p>
        <p className={`font-mono text-4xl font-bold ${textColor} tracking-wider`}>
          {formatted}
        </p>
        <p className={`font-mono text-xs ${labelColor} mt-2`}>
          {isExpired
            ? 'WINDOW CLOSED — VAULT SELF-DESTRUCT IMMINENT'
            : `REMAINING: ${Math.ceil(header.lockdownTimestamp / 60)} MINUTES`}
        </p>
      </div>
    </div>
  )
}
