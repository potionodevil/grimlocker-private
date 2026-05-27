import { useEffect, useRef } from 'react'
import { useGrimStore } from '../../store/useGrimStore'

export function TerminalPanel() {
  const { terminalLog, terminalOpen, setTerminalOpen } = useGrimStore()
  const scrollRef = useRef(null)

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [terminalLog])

  return (
    <div className="border-t border-cyber-border/50 bg-cyber-dark/50">
      <button
        onClick={() => setTerminalOpen(!terminalOpen)}
        className="w-full px-4 py-2 font-mono text-xs text-cyber-cyan/70 hover:text-cyber-cyan text-left"
      >
        {terminalOpen ? '▼' : '▶'} TERMINAL LOG
      </button>

      {terminalOpen && (
        <div
          ref={scrollRef}
          className="h-32 overflow-y-auto bg-cyber-black p-3 border-t border-cyber-border/50 font-mono text-xs text-cyber-cyan/60 space-y-1"
        >
          {terminalLog.length === 0 ? (
            <div className="text-cyber-cyan/40">Waiting for logs...</div>
          ) : (
            terminalLog.map((line, i) => (
              <div key={i} className="text-cyber-cyan/70">
                &gt; {line}
              </div>
            ))
          )}
        </div>
      )}
    </div>
  )
}
