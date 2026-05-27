import { useState } from 'react'

export function CoordinateInput({ coordinates, onChange }) {
  const [newCoord, setNewCoord] = useState({ block: '', line: '', char_index: '' })

  const addCoordinate = () => {
    const block = parseInt(newCoord.block, 10)
    const line = parseInt(newCoord.line, 10)
    const char_index = parseInt(newCoord.char_index, 10)

    if (isNaN(block) || isNaN(line) || isNaN(char_index)) return
    if (block < 0 || line < 0 || char_index < 0) return

    const updated = [...coordinates, { block, line, char_index }]
    onChange(updated)
    setNewCoord({ block: '', line: '', char_index: '' })
  }

  const removeCoordinate = (index) => {
    const updated = coordinates.filter((_, i) => i !== index)
    onChange(updated)
  }

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      addCoordinate()
    }
  }

  return (
    <div className="rounded-sm border border-cyber-border bg-cyber-dark/60 overflow-hidden">
      <div className="flex items-center justify-between px-4 py-2 border-b border-cyber-border/50">
        <span className="font-mono text-xs text-cyber-cyanDim uppercase tracking-wider">
          COORDINATES
        </span>
        <span className="font-mono text-xs text-cyber-cyan">
          [{coordinates.length}]
        </span>
      </div>

      <div className="p-4">
        {coordinates.length > 0 && (
          <div className="space-y-1 mb-4 max-h-40 overflow-auto">
            {coordinates.map((coord, index) => (
              <div
                key={index}
                className="flex items-center justify-between font-mono text-xs px-3 py-1.5 rounded-sm bg-cyber-panel/50 border border-cyber-border/30"
              >
                <span className="text-cyber-cyan">
                  [{coord.block}, {coord.line}, {coord.char_index}]
                </span>
                <button
                  onClick={() => removeCoordinate(index)}
                  className="text-cyber-redDim hover:text-cyber-red transition-colors px-2"
                  aria-label="Remove coordinate"
                >
                  ×
                </button>
              </div>
            ))}
          </div>
        )}

        <div className="flex items-center gap-2">
          <input
            type="number"
            min="0"
            placeholder="BLK"
            value={newCoord.block}
            onChange={(e) => setNewCoord({ ...newCoord, block: e.target.value })}
            onKeyDown={handleKeyDown}
            className="w-16 px-2 py-1.5 font-mono text-xs bg-cyber-black border border-cyber-border rounded-sm text-cyber-cyan placeholder-cyber-borderLight focus:border-cyber-cyan focus:outline-none transition-colors"
          />
          <input
            type="number"
            min="0"
            placeholder="LIN"
            value={newCoord.line}
            onChange={(e) => setNewCoord({ ...newCoord, line: e.target.value })}
            onKeyDown={handleKeyDown}
            className="w-16 px-2 py-1.5 font-mono text-xs bg-cyber-black border border-cyber-border rounded-sm text-cyber-cyan placeholder-cyber-borderLight focus:border-cyber-cyan focus:outline-none transition-colors"
          />
          <input
            type="number"
            min="0"
            placeholder="CHR"
            value={newCoord.char_index}
            onChange={(e) => setNewCoord({ ...newCoord, char_index: e.target.value })}
            onKeyDown={handleKeyDown}
            className="w-16 px-2 py-1.5 font-mono text-xs bg-cyber-black border border-cyber-border rounded-sm text-cyber-cyan placeholder-cyber-borderLight focus:border-cyber-cyan focus:outline-none transition-colors"
          />
          <button
            onClick={addCoordinate}
            disabled={!newCoord.block || !newCoord.line || !newCoord.char_index}
            className="px-3 py-1.5 font-mono text-xs bg-cyber-cyan/10 text-cyber-cyan border border-cyber-cyan/30 rounded-sm hover:bg-cyber-cyan/20 disabled:opacity-30 disabled:cursor-not-allowed transition-all"
          >
            +
          </button>
        </div>
      </div>
    </div>
  )
}
