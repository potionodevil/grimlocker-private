import { useState, useEffect, useRef } from 'react'
import gsap from 'gsap'
import { useGrimStore } from '../../store/useGrimStore'
import { CoordinateInput } from './CoordinateInput'
import { CountdownTimer } from './CountdownTimer'
import { EntropyDisplay } from './EntropyDisplay'
import { ScanLine } from '../shared/ScanLine'

export function LockdownScreen() {
  const { header, isLockdown, isCritical, unlock, error } = useGrimStore()
  const [coordinates, setCoordinates] = useState([])
  const [isScanning, setIsScanning] = useState(false)

  const containerRef = useRef(null)
  const titleRef = useRef(null)
  const buttonRef = useRef(null)

  useEffect(() => {
    const ctx = gsap.context(() => {
      gsap.fromTo(titleRef.current, {
        opacity: 0,
        y: -20,
      }, {
        opacity: 1,
        y: 0,
        duration: 0.8,
        ease: 'power3.out',
      })

      gsap.fromTo(buttonRef.current, {
        opacity: 0,
        scale: 0.95,
      }, {
        opacity: 1,
        scale: 1,
        duration: 0.6,
        delay: 0.3,
        ease: 'power3.out',
      })
    }, containerRef)

    return () => ctx.revert()
  }, [])

  const handleUnlock = () => {
    if (coordinates.length === 0) return

    setIsScanning(true)

    const ctx = gsap.context(() => {
      const tl = gsap.timeline({
        onComplete: () => {
          unlock(coordinates)
          setIsScanning(false)
        },
      })

      tl.to(buttonRef.current, {
        scale: 0.98,
        duration: 0.1,
        ease: 'power2.in',
      })
      .to(buttonRef.current, {
        scale: 1,
        duration: 0.15,
        ease: 'power2.out',
      })
      .to(containerRef.current, {
        boxShadow: isCritical
          ? '0 0 60px rgba(255, 51, 68, 0.3)'
          : '0 0 60px rgba(0, 240, 255, 0.2)',
        duration: 0.3,
      })
      .to(containerRef.current, {
        boxShadow: 'none',
        duration: 0.5,
      })
    }, containerRef)

    return () => ctx.revert()
  }

  const borderColor = isCritical
    ? 'border-cyber-red'
    : isLockdown
      ? 'border-cyber-amber'
      : 'border-cyber-border'

  const glowClass = isCritical
    ? 'animate-pulse-glow-red'
    : isLockdown
      ? 'animate-pulse-glow-amber'
      : 'animate-pulse-glow'

  const titleColor = isCritical
    ? 'text-cyber-red'
    : isLockdown
      ? 'text-cyber-amber'
      : 'text-cyber-cyan'

  const buttonDisabled = coordinates.length === 0 || isScanning

  return (
    <div
      ref={containerRef}
      className="min-h-screen bg-cyber-black flex items-center justify-center p-6 relative overflow-hidden"
      style={{
        backgroundImage: 'linear-gradient(rgba(42, 42, 62, 0.15) 1px, transparent 1px), linear-gradient(90deg, rgba(42, 42, 62, 0.15) 1px, transparent 1px)',
        backgroundSize: '40px 40px',
      }}
    >
      <ScanLine active={isLockdown} color={isCritical ? 'red' : 'amber'} />

      <div className={`w-full max-w-2xl ${glowClass} rounded-sm border ${borderColor} bg-cyber-dark/90 backdrop-blur-sm p-8`}>
        <div ref={titleRef} className="text-center mb-8">
          <h1 className={`font-mono text-2xl font-bold ${titleColor} tracking-wider mb-2`}>
            GRIMLOCKER
          </h1>
          <p className="font-mono text-xs text-cyber-borderLight uppercase tracking-widest">
            {isCritical
              ? 'CRITICAL LOCKDOWN — ALL OVERRIDE ATTEMPTS EXHAUSTED'
              : isLockdown
                ? 'STAGE 2 LOCKDOWN — COORDINATE AUTHENTICATION REQUIRED'
                : 'VAULT LOCKED — ENTER COORDINATES TO UNLOCK'}
          </p>
        </div>

        {isLockdown && (
          <div className="mb-6">
            <CountdownTimer />
          </div>
        )}

        <div className="mb-6">
          <EntropyDisplay />
        </div>

        <div className="mb-6">
          <CoordinateInput coordinates={coordinates} onChange={setCoordinates} />
        </div>

        {isLockdown && (
          <div className="mb-4 px-4 py-2 rounded-sm bg-cyber-amber/5 border border-cyber-amber/20">
            <p className="font-mono text-xs text-cyber-amberDim">
              OVERRIDE ATTEMPTS REMAINING: {header.overrideAttemptsLeft}/4
            </p>
          </div>
        )}

        <div ref={buttonRef} className="flex justify-center">
          <button
            onClick={handleUnlock}
            disabled={buttonDisabled}
            className={`
              font-mono text-sm px-8 py-3 rounded-sm border transition-all duration-200
              ${buttonDisabled
                ? 'border-cyber-border/30 text-cyber-borderLight/30 cursor-not-allowed bg-cyber-dark/50'
                : isCritical
                  ? 'border-cyber-red/50 text-cyber-red hover:bg-cyber-red/10 hover:border-cyber-red'
                  : isLockdown
                    ? 'border-cyber-amber/50 text-cyber-amber hover:bg-cyber-amber/10 hover:border-cyber-amber'
                    : 'border-cyber-cyan/50 text-cyber-cyan hover:bg-cyber-cyan/10 hover:border-cyber-cyan'}
            `}
          >
            {isScanning ? 'VERIFYING...' : 'UNLOCK VAULT'}
          </button>
        </div>

        {error && (
          <div className="mt-4 text-center">
            <p className="font-mono text-xs text-cyber-red">
              {error}
            </p>
          </div>
        )}
      </div>
    </div>
  )
}
