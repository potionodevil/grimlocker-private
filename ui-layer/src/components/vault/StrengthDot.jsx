export function StrengthDot({ score = 0 }) {
  // score: 0–5
  const color = score <= 2 ? 'var(--danger)' : score === 3 ? 'var(--warning)' : 'var(--success)'

  return (
    <span className="inline-flex items-center gap-0.5" aria-label={`Strength ${score}/5`}>
      {Array.from({ length: 5 }, (_, i) => (
        <span
          key={i}
          className="w-2 h-2 rounded-full"
          style={{ backgroundColor: i < score ? color : 'var(--border-strong)' }}
        />
      ))}
    </span>
  )
}
