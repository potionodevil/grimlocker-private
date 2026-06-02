import { useState, useCallback, useMemo } from 'react'
import { useCopyToClipboard } from '../../hooks/useClipboard'

const CHARSETS = {
  lowercase: 'abcdefghijklmnopqrstuvwxyz',
  uppercase: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ',
  digits: '0123456789',
  symbols: '!@#$%^&*()_+-=[]{}|;:,.<>?',
  hex: '0123456789abcdef',
}

export function CryptoGenerator() {
  const [length, setLength] = useState(24)
  const [options, setOptions] = useState({
    lowercase: true,
    uppercase: true,
    digits: true,
    symbols: false,
  })
  const [generated, setGenerated] = useState('')
  const [copied, setCopied] = useState(false)
  const copy = useCopyToClipboard()

  const charset = useMemo(() => {
    let chars = ''
    if (options.lowercase) chars += CHARSETS.lowercase
    if (options.uppercase) chars += CHARSETS.uppercase
    if (options.digits) chars += CHARSETS.digits
    if (options.symbols) chars += CHARSETS.symbols
    return chars || CHARSETS.lowercase
  }, [options])

  const generate = useCallback(() => {
    const array = new Uint32Array(length)
    crypto.getRandomValues(array)
    let result = ''
    for (let i = 0; i < length; i++) {
      result += charset[array[i] % charset.length]
    }
    setGenerated(result)
    setCopied(false)
  }, [length, charset])

  const copyToClipboard = async () => {
    if (!generated) return
    await copy(generated)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const entropy = Math.floor(length * Math.log2(charset.length))

  const toggleOption = (key) => {
    setOptions(prev => ({ ...prev, [key]: !prev[key] }))
  }

  return (
    <div className="h-full flex flex-col p-4">
      <div className="flex items-center justify-between mb-4">
        <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
          CRYPTO GENERATOR
        </span>
        <span className="font-mono text-xs text-cyber-greenDim">
          {entropy} bits entropy
        </span>
      </div>

      <div className="flex items-center gap-4 mb-4">
        <label className="font-mono text-xs text-cyber-borderLight">LENGTH</label>
        <input
          type="range"
          min="8"
          max="64"
          value={length}
          onChange={(e) => setLength(parseInt(e.target.value, 10))}
          className="flex-1 h-1 bg-cyber-dark rounded-full appearance-none cursor-pointer accent-cyber-cyan"
        />
        <span className="font-mono text-xs text-cyber-cyan w-8 text-right">{length}</span>
      </div>

      <div className="flex items-center gap-3 mb-4 flex-wrap">
        {Object.entries(options).map(([key, enabled]) => (
          <button
            key={key}
            onClick={() => toggleOption(key)}
            className={`font-mono text-xs px-3 py-1 rounded-sm border transition-all ${
              enabled
                ? 'border-cyber-cyan/50 text-cyber-cyan bg-cyber-cyan/10'
                : 'border-cyber-border/30 text-cyber-borderLight bg-cyber-dark/50'
            }`}
          >
            {key}
          </button>
        ))}
      </div>

      <div className="flex items-center gap-2 mb-4">
        <button
          onClick={generate}
          className="font-mono text-xs px-4 py-2 rounded-sm border border-cyber-cyan/50 text-cyber-cyan hover:bg-cyber-cyan/10 transition-all"
        >
          GENERATE
        </button>
        {generated && (
          <button
            onClick={copyToClipboard}
            className={`font-mono text-xs px-4 py-2 rounded-sm border transition-all ${
              copied
                ? 'border-cyber-green/50 text-cyber-green bg-cyber-green/10'
                : 'border-cyber-border/50 text-cyber-borderLight hover:bg-cyber-panel/50'
            }`}
          >
            {copied ? 'COPIED' : 'COPY'}
          </button>
        )}
      </div>

      {generated && (
        <div className="flex-1 rounded-sm border border-cyber-border/30 bg-cyber-black p-3 overflow-auto">
          <code className="font-mono text-sm text-cyber-cyan break-all">
            {generated}
          </code>
        </div>
      )}

      {!generated && (
        <div className="flex-1 rounded-sm border border-cyber-border/20 bg-cyber-dark/30 flex items-center justify-center">
          <span className="font-mono text-xs text-cyber-borderLight">
            Click GENERATE to create a secure password
          </span>
        </div>
      )}
    </div>
  )
}
