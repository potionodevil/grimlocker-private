import { useEffect, useRef } from 'react'
import gsap from 'gsap'

export function ZeroizeBar({ progress = 100, onComplete }) {
  const barRef = useRef(null)
  const progressRef = useRef(progress)

  useEffect(() => {
    if (!barRef.current) return

    progressRef.current = progress

    gsap.to(barRef.current, {
      width: `${progress}%`,
      duration: 0.3,
      ease: 'power2.out',
      onComplete: () => {
        if (progress <= 0 && onComplete) {
          onComplete()
        }
      },
    })
  }, [progress, onComplete])

  const getColor = () => {
    if (progress > 60) return 'bg-cyber-cyan'
    if (progress > 30) return 'bg-cyber-amber'
    return 'bg-cyber-red'
  }

  const getGlow = () => {
    if (progress > 60) return 'shadow-[0_0_8px_rgba(0,240,255,0.4)]'
    if (progress > 30) return 'shadow-[0_0_8px_rgba(255,170,0,0.4)]'
    return 'shadow-[0_0_8px_rgba(255,51,68,0.4)]'
  }

  return (
    <div className="w-full h-1 bg-cyber-dark rounded-full overflow-hidden">
      <div
        ref={barRef}
        className={`h-full ${getColor()} ${getGlow()} transition-colors duration-300`}
        style={{ width: `${progress}%` }}
      />
    </div>
  )
}
