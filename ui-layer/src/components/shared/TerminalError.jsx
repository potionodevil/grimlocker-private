import { useEffect, useRef } from 'react'
import gsap from 'gsap'

export function TerminalError({ message }) {
  const containerRef = useRef(null)

  useEffect(() => {
    if (!containerRef.current || !message) return

    gsap.fromTo(containerRef.current, {
      opacity: 0,
      y: -10,
      scale: 0.98,
    }, {
      opacity: 1,
      y: 0,
      scale: 1,
      duration: 0.3,
      ease: 'power2.out',
    })

    const timer = setTimeout(() => {
      gsap.to(containerRef.current, {
        opacity: 0,
        y: -10,
        duration: 0.3,
        ease: 'power2.in',
      })
    }, 4500)

    return () => clearTimeout(timer)
  }, [message])

  if (!message) return null

  const isPanic = message.includes('PANIC')
  const borderColor = isPanic ? 'border-cyber-red' : 'border-cyber-amber'
  const textColor = isPanic ? 'text-cyber-red' : 'text-cyber-amber'
  const bgColor = isPanic ? 'bg-cyber-red/10' : 'bg-cyber-amber/10'

  return (
    <div
      ref={containerRef}
      className={`fixed top-6 left-1/2 -translate-x-1/2 z-[100] px-6 py-3 rounded-sm border ${borderColor} ${bgColor} backdrop-blur-sm`}
    >
      <div className="flex items-center gap-3">
        <span className={`${textColor} font-mono text-sm`}>
          [{isPanic ? 'CRITICAL' : 'ERROR'}]
        </span>
        <span className="font-mono text-sm text-cyber-cyan">
          {message}
        </span>
      </div>
    </div>
  )
}
