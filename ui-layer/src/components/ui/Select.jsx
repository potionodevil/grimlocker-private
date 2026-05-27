import { clsx } from 'clsx'

export function Select({ className, label, children, ...props }) {
  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label className="text-sm text-text-secondary font-medium">{label}</label>
      )}
      <select
        className={clsx(
          'h-8 px-3 pr-8 rounded-md text-base text-text-primary',
          'bg-surface-subtle border border-border',
          'focus:outline-none focus:border-accent focus:ring-1 focus:ring-accent',
          'transition-colors transition-fast appearance-none',
          'bg-[url("data:image/svg+xml,%3Csvg xmlns=\'http://www.w3.org/2000/svg\' width=\'12\' height=\'12\' viewBox=\'0 0 24 24\' fill=\'none\' stroke=\'%23999\' stroke-width=\'2\'%3E%3Cpolyline points=\'6 9 12 15 18 9\'/%3E%3C/svg%3E")] bg-no-repeat bg-[right_10px_center]',
          className,
        )}
        {...props}
      >
        {children}
      </select>
    </div>
  )
}
