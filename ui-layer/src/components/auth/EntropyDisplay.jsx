import { useState } from 'react'

export function EntropyDisplay() {
  const [expanded, setExpanded] = useState(false)

  const sampleEntropy = `Block 0:
The quick brown fox jumps over the lazy dog at midnight.
Security is not a product, but a process that requires vigilance.
Entropy is the measure of uncertainty in a system.

Block 1:
In cryptography, randomness is the foundation of all security.
A single bit of predictability can compromise an entire system.
The coordinates you choose determine the strength of your key.

Block 2:
Zero-trust architecture assumes breach and verifies every request.
Memory must be locked, scrubbed, and never written to disk.
The Grimlocker vault protects what matters most.`

  return (
    <div className="rounded-sm border border-cyber-border bg-cyber-dark/60 overflow-hidden">
      <div className="flex items-center justify-between px-4 py-2 border-b border-cyber-border/50">
        <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
          ENTROPY SOURCE
        </span>
        <button
          onClick={() => setExpanded(!expanded)}
          className="font-mono text-xs text-cyber-cyan hover:text-cyber-cyanDim transition-colors"
        >
          {expanded ? '[COLLAPSE]' : '[EXPAND]'}
        </button>
      </div>

      <div className={`font-mono text-xs text-cyber-borderLight p-4 overflow-auto transition-all duration-300 ${expanded ? 'h-48' : 'h-24'}`}>
        <pre className="whitespace-pre-wrap leading-relaxed">
          {sampleEntropy}
        </pre>
      </div>

      <div className="px-4 py-2 border-t border-cyber-border/50 flex items-center gap-4">
        <span className="font-mono text-xs text-cyber-cyanDim">
          SIZE: {sampleEntropy.length} bytes
        </span>
        <span className="font-mono text-xs text-cyber-cyanDim">
          BLOCKS: 3
        </span>
        <span className="font-mono text-xs text-cyber-greenDim">
          ENTROPY: HIGH
        </span>
      </div>
    </div>
  )
}
