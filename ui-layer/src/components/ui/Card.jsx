import { clsx } from 'clsx'

export function Card({ className, children, onClick }) {
  return (
    <div
      onClick={onClick}
      className={clsx(
        'bg-surface-base border border-border rounded-md shadow-xs',
        onClick && 'cursor-pointer hover:border-border-strong hover:shadow-sm transition-base',
        className,
      )}
    >
      {children}
    </div>
  )
}
