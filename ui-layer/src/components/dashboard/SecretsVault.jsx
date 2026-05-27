import { useState, useMemo } from 'react'
import { useGrimStore } from '../../store/useGrimStore'
import { ZeroizeBar } from '../shared/ZeroizeBar'

const MOCK_SECRETS = [
  {
    id: 'eth-validator-01',
    title: 'Ethereum Validator Key',
    category: 'Web3 Infrastructure',
    fields: {
      username: 'node_admin',
      secret: '0x4a3b2c1d9e8f7a6b5c4d3e2f1a0b9c8d7e6f5a4b',
      notes: 'Mainnet staking node — 32 ETH deposited',
    },
  },
  {
    id: 'aws-root-01',
    title: 'AWS Root Account',
    category: 'Cloud Infrastructure',
    fields: {
      username: 'root@grimlocker.io',
      secret: 'AKIAIOSFODNN7EXAMPLE',
      notes: 'Production account — MFA enabled',
    },
  },
  {
    id: 'db-prod-01',
    title: 'Production Database',
    category: 'Database',
    fields: {
      username: 'grim_admin',
      secret: 'xK9#mP2$vL5@nQ8!',
      notes: 'PostgreSQL 15 — Read replica',
    },
  },
  {
    id: 'gh-pat-01',
    title: 'GitHub Personal Token',
    category: 'Development',
    fields: {
      username: 'grimlocker-bot',
      secret: 'ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
      notes: 'CI/CD pipeline access',
    },
  },
]

export function SecretsVault() {
  const { secrets, activeSecret, zeroizeProgress, selectSecret, clearActiveSecret } = useGrimStore()
  const [search, setSearch] = useState('')

  const displaySecrets = useMemo(() => {
    const list = secrets.length > 0 ? secrets : MOCK_SECRETS
    if (!search) return list
    const q = search.toLowerCase()
    return list.filter(
      s => s.title.toLowerCase().includes(q) || s.category.toLowerCase().includes(q)
    )
  }, [secrets, search])

  const categoryColors = {
    'Web3 Infrastructure': 'text-cyber-cyan',
    'Cloud Infrastructure': 'text-cyber-green',
    'Database': 'text-cyber-amber',
    'Development': 'text-cyber-borderLight',
  }

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
        {displaySecrets.map((secret) => (
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
                {secret.title}
              </span>
              <span className={`font-mono text-xs ${categoryColors[secret.category] || 'text-cyber-borderLight'}`}>
                {secret.category}
              </span>
            </div>
            <span className="font-mono text-xs text-cyber-borderLight">
              {secret.fields.username}
            </span>
          </button>
        ))}
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
            {Object.entries(activeSecret.fields).map(([key, value]) => (
              <div key={key} className="flex items-start gap-3">
                <span className="font-mono text-xs text-cyber-amberDim uppercase w-20 shrink-0">
                  {key}:
                </span>
                <span className="font-mono text-xs text-cyber-cyan break-all">
                  {key === 'secret' ? '••••••••••••' : value}
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
