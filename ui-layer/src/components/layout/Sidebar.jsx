import { clsx } from 'clsx'
import { isDevMode } from '../../utils/devMode'

const NAV_SECTIONS = [
  {
    items: [
      { id: 'all',       label: 'All Items',   icon: '⬡' },
    ],
  },
  {
    label: 'Vault',
    items: [
      { id: 'passwords', label: 'Passwords',   icon: '🔑' },
      { id: 'ssh',       label: 'SSH Keys',    icon: '🔐' },
      { id: 'certs',     label: 'Certificates',icon: '📄' },
    ],
  },
  {
    label: 'Workspace',
    items: [
      { id: 'admin',     label: 'Admin',       icon: '🛡' },
    ],
  },
  ...(isDevMode() ? [{
    label: 'Development',
    items: [
      { id: 'debug',     label: 'Debug',       icon: '🔧' },
    ],
  }] : []),
]

export function Sidebar({ activeView, onNavigate }) {
  return (
    <aside className="w-56 shrink-0 flex flex-col bg-surface-app border-r border-border h-full">
      {/* Logo */}
      <div className="h-16 flex items-center px-4 border-b border-border shrink-0">
        <span className="text-xl font-semibold text-text-primary tracking-tight">Grimlocker</span>
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto py-3 px-2">
        {NAV_SECTIONS.map((section, si) => (
          <div key={si} className="mb-4">
            {section.label && (
              <p className="px-2 mb-1 text-sm font-medium text-text-tertiary uppercase tracking-wider">
                {section.label}
              </p>
            )}
            {section.items.map((item) => (
              <button
                key={item.id}
                onClick={() => onNavigate(item.id)}
                className={clsx(
                  'w-full flex items-center gap-2.5 px-2 h-8 rounded-md text-sm transition-fast',
                  activeView === item.id
                    ? 'bg-accent-subtle text-accent font-medium border-l-2 border-accent pl-[6px]'
                    : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
                )}
              >
                <span className="text-base leading-none">{item.icon}</span>
                {item.label}
              </button>
            ))}
          </div>
        ))}
      </nav>

      {/* Bottom actions */}
      <div className="shrink-0 border-t border-border p-2">
        <button
          onClick={() => onNavigate('preferences')}
          className={clsx(
            'w-full flex items-center gap-2.5 px-2 h-8 rounded-md text-sm transition-fast',
            activeView === 'preferences'
              ? 'bg-accent-subtle text-accent font-medium'
              : 'text-text-secondary hover:bg-surface-subtle hover:text-text-primary',
          )}
        >
          <span className="text-base">⚙</span>
          Preferences
        </button>
      </div>
    </aside>
  )
}
