import { useRef, useEffect } from 'react'
import gsap from 'gsap'

export function WelcomeScreen({ onInitialize, connected, connecting }) {
  const containerRef = useRef(null)
  const titleRef = useRef(null)
  const subtitleRef = useRef(null)
  const buttonRef = useRef(null)
  const featuresRef = useRef(null)
  const statusRef = useRef(null)

  useEffect(() => {
    const ctx = gsap.context(() => {
      const tl = gsap.timeline({ defaults: { ease: 'power3.out' } })

      tl.fromTo(titleRef.current, {
        opacity: 0,
        y: -30,
        scale: 0.95,
      }, {
        opacity: 1,
        y: 0,
        scale: 1,
        duration: 0.8,
      })
      .fromTo(subtitleRef.current, {
        opacity: 0,
        y: -15,
      }, {
        opacity: 1,
        y: 0,
        duration: 0.6,
      }, '-=0.4')
      .fromTo(featuresRef.current, {
        opacity: 0,
        y: 20,
      }, {
        opacity: 1,
        y: 0,
        duration: 0.6,
      }, '-=0.3')
      .fromTo(buttonRef.current, {
        opacity: 0,
        scale: 0.9,
      }, {
        opacity: 1,
        scale: 1,
        duration: 0.5,
      }, '-=0.2')
      .fromTo(statusRef.current, {
        opacity: 0,
      }, {
        opacity: 1,
        duration: 0.4,
      }, '-=0.1')
    }, containerRef)

    return () => ctx.revert()
  }, [])

  const features = [
    { icon: '◆', label: 'ChaCha20-Poly1305 Encryption' },
    { icon: '◆', label: 'Zero-Knowledge Architecture' },
    { icon: '◆', label: 'Memory-Locked Cryptographic Core' },
    { icon: '◆', label: 'Anti-Forensic Self-Destruct' },
  ]

  const isDisabled = !connected  // Ohne Daemon-Verbindung kann der Vault nicht initialisiert werden

  return (
    <div
      ref={containerRef}
      className="min-h-screen bg-cyber-black flex items-center justify-center p-6 relative overflow-hidden"
      style={{
        backgroundImage: 'linear-gradient(rgba(42, 42, 62, 0.12) 1px, transparent 1px), linear-gradient(90deg, rgba(42, 42, 62, 0.12) 1px, transparent 1px)',
        backgroundSize: '40px 40px',
      }}
    >
      <div className="w-full max-w-lg">
        <div className={`rounded-sm border bg-cyber-dark/90 backdrop-blur-sm p-10 transition-colors duration-500 ${
          connected
            ? 'border-cyber-cyan/50 animate-pulse-glow'
            : 'border-cyber-red/30'
        }`}>
          <div ref={titleRef} className="text-center mb-3">
            <h1 className="font-mono text-3xl font-bold text-cyber-cyan tracking-widest text-glow-cyan">
              GRIMLOCKER
            </h1>
          </div>

          <div ref={subtitleRef} className="text-center mb-8">
            <p className="font-mono text-xs text-cyber-borderLight uppercase tracking-[0.3em]">
              Zero-Trust Security Vault v0.1.0
            </p>
          </div>

          <div ref={statusRef} className="mb-6">
            <div className={`rounded-sm border px-4 py-3 flex items-center gap-3 ${
              connected
                ? 'border-cyber-green/30 bg-cyber-green/5'
                : connecting
                  ? 'border-cyber-amber/30 bg-cyber-amber/5'
                  : 'border-cyber-red/30 bg-cyber-red/5'
            }`}>
              <div className={`w-2 h-2 rounded-full shrink-0 ${
                connected
                  ? 'bg-cyber-green animate-pulse'
                  : connecting
                    ? 'bg-cyber-amber animate-pulse'
                    : 'bg-cyber-red'
              }`} />
              <span className={`font-mono text-xs ${
                connected
                  ? 'text-cyber-green'
                  : connecting
                    ? 'text-cyber-amber'
                    : 'text-cyber-red'
              }`}>
                {connected
                  ? 'DAEMON CONNECTED — ws://127.0.0.1:8374'
                  : connecting
                    ? 'CONNECTING TO DAEMON...'
                    : 'DAEMON OFFLINE — Start grimlocker.exe first'}
              </span>
            </div>
          </div>

          <div ref={featuresRef} className="space-y-3 mb-10">
            {features.map((f, i) => (
              <div
                key={i}
                className="flex items-center gap-3 px-4 py-2 rounded-sm bg-cyber-panel/30 border border-cyber-border/20"
              >
                <span className="text-cyber-cyan text-sm">{f.icon}</span>
                <span className="font-mono text-xs text-cyber-cyanDim">{f.label}</span>
              </div>
            ))}
          </div>

          <div ref={buttonRef} className="flex justify-center">
            <button
              onClick={onInitialize}
              disabled={isDisabled}
              className={`font-mono text-sm px-10 py-3 rounded-sm border transition-all duration-200 tracking-wider ${
                isDisabled
                  ? 'border-cyber-border/20 text-cyber-borderLight/30 cursor-not-allowed bg-cyber-dark/50'
                  : 'border-cyber-cyan/60 text-cyber-cyan hover:bg-cyber-cyan/10 hover:border-cyber-cyan hover:text-glow-cyan'
              }`}
            >
              {connecting ? 'CONNECTING...' : 'INITIALIZE VAULT'}
            </button>
          </div>

          {!connected && !connecting && (
            <div className="mt-6 text-center space-y-2">
              <p className="font-mono text-xs text-cyber-amberDim">
                1. Start the Go daemon: <code className="text-cyber-amber">./grimlocker.exe</code>
              </p>
              <p className="font-mono text-xs text-cyber-borderLight/50">
                2. Wait for token file creation
              </p>
              <p className="font-mono text-xs text-cyber-borderLight/50">
                3. Click INITIALIZE VAULT
              </p>
            </div>
          )}

          {connected && (
            <div className="mt-6 text-center">
              <p className="font-mono text-xs text-cyber-borderLight/50">
                Entropy matrix generation requires ~15-30 seconds
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
