import { Suspense, useEffect, useRef } from 'react'
import { Canvas } from '@react-three/fiber'
import gsap from 'gsap'
import { useGrimStore } from '../../store/useGrimStore'
import { SecretsVault } from './SecretsVault'
import { CoreNodeOrb, StatusOrb } from './CoreNodeOrb'
import { EntropyIntegrity } from './EntropyIntegrity'
import { CryptoGenerator } from './CryptoGenerator'
import { ThroughputPanel } from './ThroughputPanel'
import { OperationsLog } from './OperationsLog'

export function BentoGrid() {
  const { header, isLockdown } = useGrimStore()
  const gridRef = useRef(null)
  const tilesRef = useRef([])

  const status = isLockdown ? 'lockdown' : 'secured'

  useEffect(() => {
    const ctx = gsap.context(() => {
      const tl = gsap.timeline()

      tilesRef.current.forEach((tile, i) => {
        if (!tile) return

        tl.fromTo(tile, {
          opacity: 0,
          scale: 0.9,
          y: 20,
          clipPath: 'polygon(0% 0%, 0% 0%, 0% 100%, 0% 100%)',
        }, {
          opacity: 1,
          scale: 1,
          y: 0,
          clipPath: 'polygon(0% 0%, 100% 0%, 100% 100%, 0% 100%)',
          duration: 0.5,
          ease: 'power3.out',
        }, i * 0.1)
      })
    }, gridRef)

    return () => ctx.revert()
  }, [])

  const panelClass = 'rounded-sm border border-cyber-border/50 bg-cyber-dark/80 backdrop-blur-sm overflow-hidden animate-pulse-glow'

  return (
    <div
      ref={gridRef}
      className="min-h-screen bg-cyber-black p-6"
      style={{
        backgroundImage: 'linear-gradient(rgba(42, 42, 62, 0.1) 1px, transparent 1px), linear-gradient(90deg, rgba(42, 42, 62, 0.1) 1px, transparent 1px)',
        backgroundSize: '40px 40px',
      }}
    >
      <div className="max-w-7xl mx-auto mb-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="font-mono text-xl font-bold text-cyber-cyan tracking-wider">
              GRIMLOCKER
            </h1>
            <p className="font-mono text-xs text-cyber-borderLight uppercase tracking-widest">
              SECURED VAULT DASHBOARD
            </p>
          </div>
          <div className="flex items-center gap-4">
            <span className="font-mono text-xs text-cyber-borderLight">
              OVERRIDE: {header.overrideAttemptsLeft}/4
            </span>
            <span className="font-mono text-xs text-cyber-borderLight">
              FAILED: {header.failedAttempts}/3
            </span>
            <div className="flex items-center gap-2">
              <div className={`w-2 h-2 rounded-full ${isLockdown ? 'bg-cyber-amber animate-pulse' : 'bg-cyber-green animate-pulse'}`} />
              <span className={`font-mono text-xs ${isLockdown ? 'text-cyber-amber' : 'text-cyber-green'}`}>
                {isLockdown ? 'LOCKDOWN' : 'SECURED'}
              </span>
            </div>
          </div>
        </div>
      </div>

      <div className="max-w-7xl mx-auto grid grid-cols-4 grid-rows-4 gap-4 h-[calc(100vh-8rem)]">
        {/* Secrets Vault — 2×2, der grösste Tile, zeigt Passwort-Einträge */}
        <div
          ref={el => tilesRef.current[0] = el}
          className={`col-span-2 row-span-2 ${panelClass}`}
        >
          <SecretsVault />
        </div>

        {/* Core Node Orb — 1×1, 3D-Orb-Animation des Vault-Status */}
        <div
          ref={el => tilesRef.current[1] = el}
          className={`col-span-1 row-span-1 ${panelClass} relative`}
        >
          <Suspense fallback={
            <div className="w-full h-full flex items-center justify-center">
              <span className="font-mono text-xs text-cyber-borderLight">Loading orb...</span>
            </div>
          }>
            <Canvas camera={{ position: [0, 0, 4], fov: 50 }}>
              <StatusOrb status={status} />
            </Canvas>
          </Suspense>
        </div>

        {/* Entropy Integrity — 1×1, zeigt Entropy- und Override-Status an */}
        <div
          ref={el => tilesRef.current[2] = el}
          className={`col-span-1 row-span-1 ${panelClass}`}
        >
          <EntropyIntegrity />
        </div>

        {/* Crypto Generator — 2×1 breit, erzeugt sichere Zufallspasswörter */}
        <div
          ref={el => tilesRef.current[3] = el}
          className={`col-span-2 row-span-1 ${panelClass}`}
        >
          <CryptoGenerator />
        </div>

        {/* Throughput Panel — 1×1, Live-Diagramm der kryptografischen Durchsatzrate */}
        <div
          ref={el => tilesRef.current[4] = el}
          className={`col-span-1 row-span-1 ${panelClass}`}
        >
          <ThroughputPanel />
        </div>

        {/* Operations Log — 1×1, zeigt die letzten Daemon-Operationen an */}
        <div
          ref={el => tilesRef.current[5] = el}
          className={`col-span-1 row-span-1 ${panelClass}`}
        >
          <OperationsLog />
        </div>
      </div>
    </div>
  )
}
