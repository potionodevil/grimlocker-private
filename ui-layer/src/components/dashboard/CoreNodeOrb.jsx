import { useRef, useMemo } from 'react'
import { useFrame } from '@react-three/fiber'
import { Float, MeshDistortMaterial } from '@react-three/drei'

function StatusOrb({ status }) {
  const meshRef = useRef()
  const lightRef = useRef()

  const colors = useMemo(() => ({
    secured: { main: '#00ff88', emissive: '#00aa55', intensity: 2 },
    lockdown: { main: '#ffaa00', emissive: '#aa7700', intensity: 1.5 },
    error: { main: '#ff3344', emissive: '#aa2233', intensity: 2.5 },
  }), [])

  const color = colors[status] || colors.secured

  useFrame((state, delta) => {
    if (meshRef.current) {
      meshRef.current.rotation.y += delta * 0.4
      meshRef.current.rotation.x += delta * 0.15
    }
    if (lightRef.current) {
      lightRef.current.intensity = color.intensity + Math.sin(state.clock.elapsedTime * 2) * 0.3
    }
  })

  return (
    <Float speed={1.5} rotationIntensity={0.3} floatIntensity={0.2}>
      <mesh ref={meshRef}>
        <icosahedronGeometry args={[1, 2]} />
        <MeshDistortMaterial
          color={color.main}
          emissive={color.emissive}
          emissiveIntensity={0.5}
          wireframe
          distort={0.3}
          speed={2}
        />
      </mesh>
      <pointLight ref={lightRef} color={color.main} distance={6} decay={2} />
      <ambientLight intensity={0.1} />
    </Float>
  )
}

export function CoreNodeOrb({ status = 'secured' }) {
  return (
    <div className="w-full h-full">
      <div className="absolute top-3 left-3 z-10">
        <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
          CORE NODE
        </span>
      </div>
      <div className="absolute bottom-3 left-3 z-10">
        <span className={`font-mono text-xs uppercase tracking-wider ${
          status === 'secured' ? 'text-cyber-green' :
          status === 'lockdown' ? 'text-cyber-amber' : 'text-cyber-red'
        }`}>
          {status === 'secured' ? 'CONNECTED' :
           status === 'lockdown' ? 'LOCKDOWN' : 'ERROR'}
        </span>
      </div>
    </div>
  )
}

export { StatusOrb }
