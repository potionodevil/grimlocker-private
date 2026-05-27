export function Toggle({ checked, onChange, label }) {
  return (
    <label className="inline-flex items-center gap-2 cursor-pointer select-none">
      <button
        role="switch"
        aria-checked={checked}
        onClick={() => onChange?.(!checked)}
        className="relative inline-flex w-9 h-5 rounded-full transition-colors transition-base focus:outline-none focus-visible:ring-2 focus-visible:ring-accent"
        style={{ backgroundColor: checked ? 'var(--accent)' : 'var(--border-strong)' }}
      >
        <span
          className="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow-xs transition-transform transition-base"
          style={{ transform: checked ? 'translateX(16px)' : 'translateX(0)' }}
        />
      </button>
      {label && <span className="text-sm text-text-primary">{label}</span>}
    </label>
  )
}
