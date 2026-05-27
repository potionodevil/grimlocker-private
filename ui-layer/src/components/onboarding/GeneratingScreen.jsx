import { useRef, useEffect, useState, useMemo } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import { OrbitControls } from '@react-three/drei'
import gsap from 'gsap'

function EntropySphere({ progress }) {
  const meshRef = useRef(null)
  const rotationSpeed = useMemo(() => {
    const base = 0.002
    const max = 0.05
    return base + (progress / 100) * (max - base)
  }, [progress])

  useFrame((state, delta) => {
    if (meshRef.current) {
      meshRef.current.rotation.x += rotationSpeed * delta * 60
      meshRef.current.rotation.y += rotationSpeed * 1.5 * delta * 60
    }
  })

  const color = progress < 30 ? '#00f0ff' : progress < 70 ? '#ffaa00' : '#00ff88'
  const emissiveIntensity = 0.3 + (progress / 100) * 0.7

  return (
    <group>
      <mesh ref={meshRef}>
        <icosahedronGeometry args={[1.2, 3]} />
        <meshBasicMaterial
          color={color}
          wireframe
          transparent
          opacity={0.6 + (progress / 100) * 0.4}
        />
      </mesh>
      <mesh>
        <icosahedronGeometry args={[1.4, 2]} />
        <meshBasicMaterial
          color={color}
          wireframe
          transparent
          opacity={0.15}
        />
      </mesh>
      <pointLight position={[0, 0, 0]} intensity={emissiveIntensity} color={color} />
      <ambientLight intensity={0.2} />
    </group>
  )
}

export function GeneratingScreen({ progress, stage, message }) {
  const containerRef = useRef(null)
  const barRef = useRef(null)
  const percentRef = useRef(null)
  const stageRef = useRef(null)
  const messageRef = useRef(null)

  const [particles, setParticles] = useState([])

  useEffect(() => {
    const newParticles = Array.from({ length: 40 }, (_, i) => ({
      id: i,
      x: Math.random() * 100,
      y: Math.random() * 100,
      size: Math.random() * 2 + 1,
      speed: Math.random() * 0.5 + 0.2,
      opacity: Math.random() * 0.3 + 0.1,
    }))
    setParticles(newParticles)
  }, [])

  useEffect(() => {
    const ctx = gsap.context(() => {
      gsap.fromTo(containerRef.current, {
        opacity: 0,
        scale: 0.95,
      }, {
        opacity: 1,
        scale: 1,
        duration: 0.6,
        ease: 'power3.out',
      })
    }, containerRef)

    return () => ctx.revert()
  }, [])

  useEffect(() => {
    if (barRef.current) {
      gsap.to(barRef.current, {
        width: `${progress}%`,
        duration: 0.4,
        ease: 'power2.out',
      })
    }

    if (percentRef.current) {
      percentRef.current.textContent = `${progress}%`
    }

    if (stageRef.current) {
      gsap.fromTo(stageRef.current, {
        opacity: 0.5,
      }, {
        opacity: 1,
        duration: 0.3,
      })
      stageRef.current.textContent = stage.toUpperCase()
    }

    if (messageRef.current) {
      messageRef.current.textContent = message
    }
  }, [progress, stage, message])

  const getBarColor = () => {
    if (progress < 30) return 'bg-cyber-cyan'
    if (progress < 70) return 'bg-cyber-amber'
    return 'bg-cyber-green'
  }

  const getGlowColor = () => {
    if (progress < 30) return 'shadow-[0_0_12px_rgba(0,240,255,0.5)]'
    if (progress < 70) return 'shadow-[0_0_12px_rgba(255,170,0,0.5)]'
    return 'shadow-[0_0_12px_rgba(0,255,136,0.5)]'
  }

  return (
    <div
      ref={containerRef}
      className="min-h-screen bg-cyber-black flex items-center justify-center p-6 relative overflow-hidden"
      style={{
        backgroundImage: 'linear-gradient(rgba(42, 42, 62, 0.12) 1px, transparent 1px), linear-gradient(90deg, rgba(42, 42, 62, 0.12) 1px, transparent 1px)',
        backgroundSize: '40px 40px',
      }}
    >
      {particles.map(p => (
        <div
          key={p.id}
          className="absolute rounded-full bg-cyber-cyan/20"
          style={{
            left: `${p.x}%`,
            top: `${p.y}%`,
            width: `${p.size}px`,
            height: `${p.size}px`,
            opacity: p.opacity,
            animation: `pulse ${2 / p.speed}s ease-in-out infinite`,
          }}
        />
      ))}

      <div className="w-full max-w-md">
        <div className="rounded-sm border border-cyber-border/50 bg-cyber-dark/90 backdrop-blur-sm p-10">
          <div className="text-center mb-8">
            <h2 className="font-mono text-xl font-bold text-cyber-cyan tracking-wider mb-2">
              GENERATING ENTROPY MATRIX
            </h2>
            <p className="font-mono text-xs text-cyber-borderLight">
              400,000 lines of cryptographic randomness
            </p>
          </div>

          <div className="mb-6">
            <div className="flex justify-center mb-6 h-48">
              <Canvas camera={{ position: [0, 0, 4], fov: 50 }}>
                <EntropySphere progress={progress} />
                <OrbitControls enableZoom={false} enablePan={false} enableRotate={false} />
              </Canvas>
            </div>
          </div>

          <div className="mb-4">
            <div className="w-full h-2 bg-cyber-black rounded-full overflow-hidden border border-cyber-border/30">
              <div
                ref={barRef}
                className={`h-full ${getBarColor()} ${getGlowColor()} transition-colors duration-500`}
                style={{ width: '0%' }}
              />
            </div>
          </div>

          <div className="flex items-center justify-between mb-2">
            <span ref={stageRef} className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
              INITIALIZING
            </span>
            <span className="font-mono text-xs text-cyber-borderLight">
              400K lines
            </span>
          </div>

          <div className="rounded-sm bg-cyber-black/50 border border-cyber-border/20 px-4 py-2">
            <p ref={messageRef} className="font-mono text-xs text-cyber-borderLight">
              {message || 'Starting...'}
            </p>
          </div>

          <div className="mt-6 flex items-center justify-center gap-2">
            <div className="w-1.5 h-1.5 rounded-full bg-cyber-cyan animate-pulse" />
            <div className="w-1.5 h-1.5 rounded-full bg-cyber-cyan/50 animate-pulse" style={{ animationDelay: '0.2s' }} />
            <div className="w-1.5 h-1.5 rounded-full bg-cyber-cyan/25 animate-pulse" style={{ animationDelay: '0.4s' }} />
          </div>
        </div>
      </div>
    </div>
  )
}
