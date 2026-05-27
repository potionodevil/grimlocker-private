import { clsx } from 'clsx'

const variants = {
  success: 'bg-success-subtle text-success',
  warning: 'bg-warning-subtle text-warning',
  danger:  'bg-danger-subtle  text-danger',
  neutral: 'bg-surface-subtle text-text-secondary',
  accent:  'bg-accent-subtle  text-accent',
}

export function Badge({ variant = 'neutral', className, children }) {
  return (
    <span
      className={clsx(
        'inline-flex items-center px-2 py-0.5 rounded-sm text-sm font-medium',
        variants[variant],
        className,
      )}
    >
      {children}
    </span>
  )
}
