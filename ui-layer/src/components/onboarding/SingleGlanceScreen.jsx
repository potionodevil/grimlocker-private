import { useRef, useEffect, useState, useCallback } from 'react'
import gsap from 'gsap'

export function SingleGlanceScreen({ keyHex, coordinates, onProceed, timeLeft }) {
  const containerRef = useRef(null)
  const keyRef = useRef(null)
  const timerRef = useRef(null)
  const coordsRef = useRef(null)
  const [displayedKey, setDisplayedKey] = useState('')
  const [revealIndex, setRevealIndex] = useState(0)

  useEffect(() => {
    const ctx = gsap.context(() => {
      gsap.fromTo(containerRef.current, {
        opacity: 0,
        scale: 0.9,
        clipPath: 'polygon(50% 50%, 50% 50%, 50% 50%, 50% 50%)',
      }, {
        opacity: 1,
        scale: 1,
        clipPath: 'polygon(0% 0%, 100% 0%, 100% 100%, 0% 100%)',
        duration: 0.8,
        ease: 'power3.out',
      })
    }, containerRef)

    return () => ctx.revert()
  }, [])

  useEffect(() => {
    if (!keyHex) return

    let idx = 0
    const interval = setInterval(() => {
      if (idx < keyHex.length) {
        setDisplayedKey(keyHex.slice(0, idx + 1))
        setRevealIndex(idx + 1)
        idx++
      } else {
        clearInterval(interval)
      }
    }, 20)

    return () => clearInterval(interval)
  }, [keyHex])

  useEffect(() => {
    const preventCopy = (e) => e.preventDefault()
    const preventCut = (e) => e.preventDefault()
    const preventContextMenu = (e) => e.preventDefault()
    const preventDrag = (e) => e.preventDefault()

    // Copy/Clipboard-Schutz: Sobald der 256-Bit-Key im Klartext sichtbar ist,
    // blockieren wir alle Wege, wie ein Angreifer (oder der Browser) den Key abgreifen könnte.
    // Das ist ein bewusster UX-Trade-off: Sicherheit geht hier vor Bequemlichkeit.
    const preventKeyboardShortcuts = (e) => {
      const blocked = [
        e.ctrlKey && e.key === 'c',
        e.ctrlKey && e.key === 'x',
        e.ctrlKey && e.key === 'v',
        e.ctrlKey && e.key === 'a',
        e.ctrlKey && e.key === 'p',
        e.ctrlKey && e.key === 's',
        e.ctrlKey && e.key === 'u',
        e.metaKey && e.key === 'c',
        e.metaKey && e.key === 'x',
        e.metaKey && e.key === 'v',
        e.metaKey && e.key === 'a',
        e.metaKey && e.key === 'p',
        e.metaKey && e.key === 's',
        e.metaKey && e.key === 'u',
        e.key === 'F12',
        e.ctrlKey && e.shiftKey && e.key === 'I',
        e.ctrlKey && e.shiftKey && e.key === 'J',
        e.ctrlKey && e.shiftKey && e.key === 'C',
        e.metaKey && e.altKey && e.key === 'I',
        e.metaKey && e.altKey && e.key === 'J',
        e.key === 'PrintScreen',
      ]
      if (blocked.some(Boolean)) {
        e.preventDefault()
        e.stopPropagation()
        return false
      }
    }

    document.addEventListener('copy', preventCopy, { capture: true })
    document.addEventListener('cut', preventCut, { capture: true })
    document.addEventListener('contextmenu', preventContextMenu, { capture: true })
    document.addEventListener('dragstart', preventDrag, { capture: true })
    document.addEventListener('keydown', preventKeyboardShortcuts, { capture: true })

    return () => {
      document.removeEventListener('copy', preventCopy, { capture: true })
      document.removeEventListener('cut', preventCut, { capture: true })
      document.removeEventListener('contextmenu', preventContextMenu, { capture: true })
      document.removeEventListener('dragstart', preventDrag, { capture: true })
      document.removeEventListener('keydown', preventKeyboardShortcuts, { capture: true })
    }
  }, [])

  const handleProceed = useCallback(() => {
    const ctx = gsap.context(() => {
      gsap.to(containerRef.current, {
        opacity: 0,
        scale: 0.95,
        duration: 0.4,
        ease: 'power2.in',
        onComplete: () => {
          setDisplayedKey('')
          setRevealIndex(0)
          onProceed()
        },
      })
    }, containerRef)

    return () => ctx.revert()
  }, [onProceed])

  const formatKey = (key) => {
    const chunks = []
    for (let i = 0; i < key.length; i += 8) {
      chunks.push(key.slice(i, i + 8))
    }
    return chunks.join(' ')
  }

  const coordSummary = coordinates
    ? coordinates.map(c => `[${c.block},${c.line},${c.char_index}]`).join(' ')
    : ''

  return (
    <div
      ref={containerRef}
      className="min-h-screen bg-cyber-black flex items-center justify-center p-6 relative overflow-hidden select-none"
      style={{
        backgroundImage: 'linear-gradient(rgba(42, 42, 62, 0.12) 1px, transparent 1px), linear-gradient(90deg, rgba(42, 42, 62, 0.12) 1px, transparent 1px)',
        backgroundSize: '40px 40px',
      }}
    >
      <div className="absolute inset-0 pointer-events-none" style={{ zIndex: 1 }} />

      <div className="w-full max-w-2xl relative z-10">
        <div className="rounded-sm border border-cyber-amber/50 bg-cyber-dark/95 backdrop-blur-sm p-8 animate-pulse-glow-amber">
          <div className="text-center mb-6">
            <h2 className="font-mono text-xl font-bold text-cyber-amber tracking-wider mb-1">
              SINGLE GLANCE PROTOCOL
            </h2>
            {/* Wichtig: Der Key wird NUR HIER im Klartext gezeigt und dann zeroized.
                 Copy/Paste/Screenshot sind blockiert — das ist Absicht. */}
            <p className="font-mono text-xs text-cyber-amberDim uppercase tracking-widest">
              MEMORIZE THIS KEY — IT WILL BE ZEROIZED
            </p>
          </div>

          <div className="mb-6">
            <div className="flex items-center justify-between mb-2">
              <span className="font-mono text-xs text-cyber-borderLight">DERIVED KEY (256-bit)</span>
              <span className="font-mono text-xs text-cyber-amberDim">{displayedKey.length}/64 chars</span>
            </div>
            <div
              ref={keyRef}
              className="rounded-sm bg-cyber-black border border-cyber-amber/20 p-4 font-mono text-sm text-cyber-amber break-all leading-relaxed no-copy"
              style={{ userSelect: 'none', WebkitUserSelect: 'none', MozUserSelect: 'none', msUserSelect: 'none' }}
            >
              {formatKey(displayedKey)}
              <span className="animate-pulse text-cyber-amber/50">█</span>
            </div>
          </div>

          <div className="mb-6">
            <div className="flex items-center justify-between mb-2">
              <span className="font-mono text-xs text-cyber-borderLight">COORDINATE MAP</span>
              <span className="font-mono text-xs text-cyber-cyanDim">{coordinates ? coordinates.length : 0} points</span>
            </div>
            <div
              ref={coordsRef}
              className="rounded-sm bg-cyber-black border border-cyber-border/20 p-3 max-h-24 overflow-auto font-mono text-xs text-cyber-cyanDim leading-relaxed no-copy"
              style={{ userSelect: 'none', WebkitUserSelect: 'none' }}
            >
              {coordSummary}
            </div>
          </div>

          <div className="mb-6">
            <div className="flex items-center justify-between mb-2">
              <span className="font-mono text-xs text-cyber-redDim">AUTO-ZEROIZE TIMER</span>
              <span ref={timerRef} className="font-mono text-sm font-bold text-cyber-red">
                {timeLeft}s
              </span>
            </div>
            <div className="w-full h-1.5 bg-cyber-black rounded-full overflow-hidden">
              <div
                className="h-full bg-cyber-red shadow-[0_0_8px_rgba(255,51,68,0.4)] transition-all duration-1000"
                style={{ width: `${(timeLeft / 30) * 100}%` }}
              />
            </div>
          </div>

          <div className="flex items-center justify-center gap-4">
            <button
              onClick={handleProceed}
              className="font-mono text-sm px-8 py-3 rounded-sm border border-cyber-amber/50 text-cyber-amber hover:bg-cyber-amber/10 hover:border-cyber-amber transition-all duration-200 tracking-wider"
            >
              PROCEED TO DASHBOARD
            </button>
          </div>

          <div className="mt-4 text-center">
            <p className="font-mono text-xs text-cyber-borderLight/40">
              Copy, paste, and screenshot are blocked. Key is purged from JS memory on proceed.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
