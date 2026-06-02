import { useState, useMemo } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { ZeroizeBar } from '../shared/ZeroizeBar'

export function SecretsVault() {
  const { entries, activeSecret, zeroizeProgress, selectSecret, clearActiveSecret } = useGrimStore()
  const [search, setSearch] = useState('')

  const displaySecrets = useMemo(() => {
    const passwordEntries = entries.filter(e =>
      e.category === 'PASSWORD' || e.type === 'password' || e.type === 'PASSWORD'
    )
    const list = passwordEntries.length > 0 ? passwordEntries : []
    if (!search) return list
    const q = search.toLowerCase()
    return list.filter(
      s => (s.title || s.name || '').toLowerCase().includes(q) || (s.category || s.type || '').toLowerCase().includes(q)
    )
  }, [entries, search])

  return (
    <div className="h-full flex flex-col">
      <div className="flex items-center justify-between px-4 py-3 border-b border-cyber-border/50">
        <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
          SECRETS VAULT
        </span>
        <span className="font-mono text-xs text-cyber-cyan">
          [{displaySecrets.length}]
        </span>
      </div>

      <div className="px-4 py-2 border-b border-cyber-border/30">
        <input
          type="text"
          placeholder="Search by title or category..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full px-3 py-1.5 font-mono text-xs bg-cyber-black border border-cyber-border rounded-sm text-cyber-cyan placeholder-cyber-borderLight focus:border-cyber-cyan focus:outline-none transition-colors"
        />
      </div>

      <div className="flex-1 overflow-auto p-4 space-y-2">
        {displaySecrets.length === 0 ? (
          <div className="text-center py-12 text-cyber-borderLight font-mono text-xs">
            No password entries yet
          </div>
        ) : (
          displaySecrets.map((secret) => (
            <button
              key={secret.id}
              onClick={() => selectSecret(secret)}
              className={`w-full text-left px-4 py-3 rounded-sm border transition-all duration-200 ${
                activeSecret?.id === secret.id
                  ? 'border-cyber-cyan bg-cyber-cyan/5'
                  : 'border-cyber-border/30 bg-cyber-panel/30 hover:border-cyber-border hover:bg-cyber-panel/50'
              }`}
            >
              <div className="flex items-center justify-between mb-1">
                <span className="font-mono text-sm text-cyber-cyan font-medium">
                  {secret.title || secret.name || 'Untitled'}
                </span>
                <span className="font-mono text-xs text-cyber-borderLight">
                  {secret.category || secret.type || ''}
                </span>
              </div>
              <span className="font-mono text-xs text-cyber-borderLight">
                {secret.username || secret.data?.username || ''}
              </span>
            </button>
          ))
        )}
      </div>

      {activeSecret && (
        <div className="border-t border-cyber-border/50 p-4 bg-cyber-dark/80">
          <div className="flex items-center justify-between mb-2">
            <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
              ACTIVE SECRET — ZEROIZE TIMER
            </span>
            <button
              onClick={clearActiveSecret}
              className="font-mono text-xs text-cyber-redDim hover:text-cyber-red transition-colors"
            >
              [CLEAR]
            </button>
          </div>

          <div className="space-y-2 mb-3">
            {Object.entries({
              title: activeSecret.title || activeSecret.name || '',
              username: activeSecret.username || activeSecret.data?.username || '',
              ...(activeSecret.data || {}),
            }).map(([key, value]) => (
              <div key={key} className="flex items-start gap-3">
                <span className="font-mono text-xs text-cyber-amberDim uppercase w-20 shrink-0">
                  {key}:
                </span>
                <span className="font-mono text-xs text-cyber-cyan break-all">
                  {key === 'password' || key === 'secret' ? '••••••••••••' : value}
                </span>
              </div>
            ))}
          </div>

          <ZeroizeBar progress={zeroizeProgress} onComplete={clearActiveSecret} />
          <div className="flex justify-between mt-1">
            <span className="font-mono text-xs text-cyber-borderLight">
              RAM LIFESPAN
            </span>
            <span className="font-mono text-xs text-cyber-cyan">
              {Math.ceil(zeroizeProgress / 100 * 30)}s
            </span>
          </div>
        </div>
      )}
    </div>
  )
}