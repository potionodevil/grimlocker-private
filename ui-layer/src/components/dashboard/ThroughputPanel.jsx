import { useGrimStore } from '../../store/useGrimStore'

export function ThroughputPanel() {
  const throughputData = useGrimStore((s) => s.throughputData)

  const W = 240
  const H = 60
  const max = Math.max(...throughputData.map((d) => d.bytes), 1)
  const pts = throughputData.map((d, i) => [
    (i / (throughputData.length - 1 || 1)) * W,
    H - (d.bytes / max) * H,
  ])
  const path = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0]},${p[1]}`).join(' ')
  const totalBytes = throughputData.reduce((a, d) => a + d.bytes, 0)

  return (
    <div className="flex flex-col gap-2 p-3 h-full">
      <div className="text-cyber-cyan/60 text-xs font-mono uppercase tracking-wider">
        Cryptographic Throughput
      </div>
      <svg width={W} height={H} className="w-full" viewBox={`0 0 ${W} ${H}`}>
        <defs>
          <linearGradient id="tg" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#00f0ff" stopOpacity="0.4" />
            <stop offset="100%" stopColor="#00f0ff" stopOpacity="0" />
          </linearGradient>
        </defs>
        {pts.length > 1 && (
          <path d={`${path} L${W},${H} L0,${H} Z`} fill="url(#tg)" />
        )}
        {pts.length > 1 && (
          <path d={path} stroke="#00f0ff" strokeWidth="1.5" fill="none" strokeLinecap="round" />
        )}
      </svg>
      <div className="text-cyber-cyan/40 text-xs font-mono">
        {(totalBytes / 1024).toFixed(1)} KB total
      </div>
    </div>
  )
}
