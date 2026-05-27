import { clsx } from 'clsx'

const variants = {
  primary:   'bg-accent text-text-inverse hover:bg-accent-hover',
  secondary: 'bg-surface-subtle text-text-primary border border-border hover:bg-[var(--border-default)]',
  ghost:     'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
  danger:    'bg-danger text-text-inverse hover:opacity-90',
}

const sizes = {
  sm: 'h-7 px-2.5 text-sm gap-1.5',
  md: 'h-8 px-3 text-base gap-2',
  lg: 'h-10 px-4 text-lg gap-2',
}

export function Button({ variant = 'primary', size = 'md', className, children, ...props }) {
  return (
    <button
      className={clsx(
        'inline-flex items-center justify-center font-medium rounded-md',
        'transition-colors transition-fast select-none',
        'disabled:opacity-50 disabled:cursor-not-allowed',
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    >
      {children}
    </button>
  )
}
