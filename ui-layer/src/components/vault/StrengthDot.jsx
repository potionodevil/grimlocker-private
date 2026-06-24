const LABELS = ['—', 'Sehr schwach', 'Schwach', 'Mittel', 'Stark', 'Sehr stark']

export function StrengthDot({ score = 0, showLabel = false }) {
  const color = score <= 2 ? 'var(--danger)' : score === 3 ? 'var(--warning)' : 'var(--success)'

  return (
    <span className="inline-flex items-center gap-1.5" aria-label={`Stärke ${score}/5`}>
      <span className="inline-flex items-center gap-0.5">
        {Array.from({ length: 5 }, (_, i) => (
          <span
            key={i}
            className="w-1.5 h-1.5 rounded-full"
            style={{ backgroundColor: i < score ? color : 'var(--border-strong)' }}
          />
        ))}
      </span>
      {showLabel && score > 0 && (
        <span className="text-[10px] tabular-nums" style={{ color }}>
          {LABELS[score] ?? score}
        </span>
      )}
    </span>
  )
}
