import { useEffect, useRef } from 'react'
import gsap from 'gsap'

export function ScanLine({ active = true, color = 'cyan' }) {
  const lineRef = useRef(null)

  useEffect(() => {
    if (!active || !lineRef.current) return

    const ctx = gsap.context(() => {
      gsap.to(lineRef.current, {
        y: '100vh',
        duration: 2.5,
        ease: 'none',
        repeat: -1,
      })
    })

    return () => ctx.revert()
  }, [active])

  const colorClasses = {
    cyan: 'bg-cyber-cyan/30 shadow-[0_0_15px_rgba(0,240,255,0.5)]',
    amber: 'bg-cyber-amber/30 shadow-[0_0_15px_rgba(255,170,0,0.5)]',
    red: 'bg-cyber-red/30 shadow-[0_0_15px_rgba(255,51,68,0.5)]',
  }

  return (
    <div
      ref={lineRef}
      className={`absolute left-0 right-0 h-px ${colorClasses[color]} pointer-events-none z-50`}
      style={{ y: '-100%' }}
    />
  )
}
