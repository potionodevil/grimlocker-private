import { clsx } from 'clsx'

export function Input({ className, label, ...props }) {
  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label className="text-sm text-text-secondary font-medium">{label}</label>
      )}
      <input
        className={clsx(
          'h-8 px-3 rounded-md text-base text-text-primary',
          'bg-surface-subtle border border-border',
          'placeholder:text-text-tertiary',
          'focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent',
          'transition-colors transition-fast',
          className,
        )}
        {...props}
      />
    </div>
  )
}
