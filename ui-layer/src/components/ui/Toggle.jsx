export function Toggle({ checked, onChange, label, description }) {
  return (
    <label className="flex items-start gap-3 cursor-pointer select-none group">
      <button
        role="switch"
        aria-checked={checked}
        onClick={() => onChange?.(!checked)}
        className="relative inline-flex w-9 h-5 rounded-full transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-accent shrink-0 mt-0.5"
        style={{ backgroundColor: checked ? 'var(--accent)' : 'var(--border-strong)' }}
      >
        <span
          className="absolute top-0.5 left-0.5 w-4 h-4 rounded-full bg-white shadow-sm transition-transform duration-200"
          style={{ transform: checked ? 'translateX(16px)' : 'translateX(0)' }}
        />
      </button>
      {(label || description) && (
        <div>
          {label && <span className="text-sm text-text-primary">{label}</span>}
          {description && <p className="text-xs text-text-tertiary mt-0.5">{description}</p>}
        </div>
      )}
    </label>
  )
}
