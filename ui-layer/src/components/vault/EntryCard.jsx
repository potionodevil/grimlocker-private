import { useState } from 'react'
import { clsx } from 'clsx'
import { useGrimStore } from '../../store/useGrimStore'

function getAccentHex() {
  const v = getComputedStyle(document.documentElement).getPropertyValue('--accent').trim()
  return v || '#4F8EF7'
}

const CATEGORY_CONFIG = {
  PASSWORD:    { label: 'PW',   color: 'text-blue-400',   bg: 'bg-blue-500/10',   useAccent: true },
  SSH_KEY:     { label: 'SSH',  color: 'text-purple-400', bg: 'bg-purple-500/10', raw: '#A855F7' },
  CERTIFICATE: { label: 'CERT', color: 'text-teal-400',   bg: 'bg-teal-500/10',   raw: '#14B8A6' },
  FILE_VAULT:  { label: 'FILE', color: 'text-orange-400', bg: 'bg-orange-500/10', raw: '#F97316' },
  TOTP:        { label: 'OTP',  color: 'text-green-400',  bg: 'bg-green-500/10',  raw: '#22C55E' },
  NOTE:        { label: 'NOTE', color: 'text-yellow-400', bg: 'bg-yellow-500/10', raw: '#EAB308' },
}
const LEGACY_CONFIG = {
  password:    CATEGORY_CONFIG.PASSWORD,
  ssh:         CATEGORY_CONFIG.SSH_KEY,
  certs:       CATEGORY_CONFIG.CERTIFICATE,
  certificate: CATEGORY_CONFIG.CERTIFICATE,
  file_vault:  CATEGORY_CONFIG.FILE_VAULT,
}

function getCfg(entry) {
  return (
    CATEGORY_CONFIG[entry.category] ||
    LEGACY_CONFIG[entry.type] ||
    { label: (entry.type || '?').slice(0, 4).toUpperCase(), color: 'text-text-tertiary', bg: 'bg-surface-subtle', raw: '#6B7280' }
  )
}

function strengthColor(score) {
  if (score <= 1) return '#EF4444'
  if (score <= 2) return '#F97316'
  if (score === 3) return '#EAB308'
  if (score === 4) return '#22C55E'
  return '#16A34A'
}

function relativeTime(nanos) {
  if (!nanos) return '—'
  const diff = Date.now() - nanos / 1e6
  const m = Math.floor(diff / 60000)
  if (m < 1)  return 'gerade eben'
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h`
  const d = Math.floor(h / 24)
  if (d < 30) return `${d}d`
  return `${Math.floor(d / 30)}mo`
}

function getSubtitle(entry) {
  return entry.fields?.username || entry.fields?.user || entry.username || entry.label || ''
}

// Icon paths per category label
const ICON_PATHS = {
  PW:   'M12 15v2m-6 4h12a2 2 0 0 0 2-2v-6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2zm10-10V7a4 4 0 0 0-8 0v4h8z',
  SSH:  'M8 9l3 3-3 3M13 15h3M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z',
  CERT: 'M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0 1 12 2.944a11.955 11.955 0 0 1-8.618 3.04A12.02 12.02 0 0 0 3 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z',
  FILE: 'M9 13h6m-3-3v6m5 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z',
  OTP:  'M12 8v4l3 3m6-3a9 9 0 1 1-18 0 9 9 0 0 1 18 0z',
  NOTE: 'M11 5H6a2 2 0 0 0-2 2v11a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2v-5m-1.414-9.414a2 2 0 1 1 2.828 2.828L11.828 15H9v-2.828l8.586-8.586z',
}

function hex(raw, alpha) {
  // Convert hex + alpha to rgba string
  const r = parseInt(raw.slice(1, 3), 16)
  const g = parseInt(raw.slice(3, 5), 16)
  const b = parseInt(raw.slice(5, 7), 16)
  return `rgba(${r},${g},${b},${alpha})`
}

// ── Grid View ─────────────────────────────────────────────────────────────────
function GridCard({ entry, onContextMenu }) {
  const [hovered, setHovered] = useState(false)
  const fetchEntry   = useGrimStore((s) => s.fetchEntry)
  const showSubtitle = useGrimStore((s) => s.preferences.showEntrySubtitle ?? true)
  const showTimestamp = useGrimStore((s) => s.preferences.showEntryTimestamp ?? true)
  const showStrength = useGrimStore((s) => s.preferences.showPasswordStrength)
  const cfg = getCfg(entry)
  const raw = cfg.useAccent ? getAccentHex() : cfg.raw
  const subtitle = getSubtitle(entry)
  const sColor = strengthColor(entry.strength ?? 0)
  const iconPath = ICON_PATHS[cfg.label] || ICON_PATHS.PW

  return (
    <div
      onClick={() => fetchEntry(entry.id)}
      onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); onContextMenu?.(e, entry) }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="relative overflow-hidden rounded-xl cursor-pointer flex flex-col"
      style={{
        background: hovered
          ? `linear-gradient(135deg, ${hex(raw, 0.10)} 0%, ${hex(raw, 0.04)} 40%, transparent 70%)`
          : `linear-gradient(135deg, ${hex(raw, 0.05)} 0%, transparent 60%)`,
        border: `1px solid ${hovered ? hex(raw, 0.35) : hex(raw, 0.15)}`,
        boxShadow: hovered
          ? `0 0 0 1px ${hex(raw, 0.2)}, 0 8px 28px ${hex(raw, 0.18)}, 0 2px 8px rgba(0,0,0,0.15)`
          : `0 1px 3px rgba(0,0,0,0.08)`,
        transform: hovered ? 'translateY(-2px) scale(1.005)' : 'translateY(0) scale(1)',
        transition: 'all 220ms cubic-bezier(0.34, 1.56, 0.64, 1)',
        padding: '16px',
        gap: '12px',
      }}
    >
      {/* Mehr-Button */}
      <button
        onClick={(e) => { e.stopPropagation(); onContextMenu?.(e, entry) }}
        className="absolute top-3 right-3 w-6 h-6 flex items-center justify-center rounded-md text-text-tertiary hover:text-text-primary transition-fast"
        style={{
          opacity: hovered ? 1 : 0,
          background: hovered ? hex(raw, 0.12) : 'transparent',
          transition: 'opacity 150ms ease',
        }}
      >
        <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor">
          <circle cx="5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="19" cy="12" r="1.5"/>
        </svg>
      </button>

      {/* Icon + Title */}
      <div className="flex items-start gap-3">
        {/* Glowing icon container */}
        <div
          className="w-9 h-9 rounded-xl flex items-center justify-center shrink-0 relative"
          style={{
            background: `linear-gradient(135deg, ${hex(raw, 0.22)}, ${hex(raw, 0.10)})`,
            boxShadow: hovered ? `0 0 12px ${hex(raw, 0.4)}` : `0 0 0px ${hex(raw, 0)}`,
            transition: 'box-shadow 220ms ease',
          }}
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor"
            strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round"
            style={{ color: raw }}>
            <path d={iconPath}/>
          </svg>
        </div>

        <div className="min-w-0 flex-1 pt-0.5">
          <p className="text-sm font-semibold text-text-primary truncate leading-tight pr-6">
            {entry.title || 'Untitled'}
          </p>
          {showSubtitle && (
            <p className="text-xs text-text-tertiary truncate mt-0.5">
              {subtitle || ' '}
            </p>
          )}
        </div>
      </div>

      {/* Footer */}
      <div className="mt-auto space-y-2">
        {showStrength && entry.strength != null && (
          <div className="h-1 rounded-full overflow-hidden" style={{ background: hex(raw, 0.12) }}>
            <div
              className="h-full rounded-full"
              style={{
                width: `${(entry.strength / 5) * 100}%`,
                background: `linear-gradient(to right, ${sColor}99, ${sColor})`,
                transition: 'width 400ms ease',
              }}
            />
          </div>
        )}
        {showTimestamp && (
          <div className="flex items-center justify-between">
            <span className="text-[10px] text-text-tertiary tabular-nums">
              {relativeTime(entry.updatedAt)}
            </span>
            {/* Glowing category badge */}
            <span
              className="text-[9px] font-mono font-bold px-1.5 py-0.5 rounded-md leading-none tracking-wide"
              style={{
                color: raw,
                background: hex(raw, 0.12),
                border: `1px solid ${hex(raw, 0.25)}`,
              }}
            >
              {cfg.label}
            </span>
          </div>
        )}
      </div>
    </div>
  )
}

// ── List View ─────────────────────────────────────────────────────────────────
function ListCard({ entry, onContextMenu, index = 0 }) {
  const [hovered, setHovered] = useState(false)
  const fetchEntry   = useGrimStore((s) => s.fetchEntry)
  const showSubtitle = useGrimStore((s) => s.preferences.showEntrySubtitle ?? true)
  const showTimestamp = useGrimStore((s) => s.preferences.showEntryTimestamp ?? true)
  const showStrength = useGrimStore((s) => s.preferences.showPasswordStrength)
  const cfg = getCfg(entry)
  const raw = cfg.useAccent ? getAccentHex() : cfg.raw
  const sColor = strengthColor(entry.strength ?? 0)
  const isEven = index % 2 === 0

  return (
    <div
      onClick={() => fetchEntry(entry.id)}
      onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); onContextMenu?.(e, entry) }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="flex items-center gap-4 h-12 pr-4 cursor-pointer border-b border-border/30"
      style={{
        background: hovered
          ? `linear-gradient(to right, ${hex(raw, 0.10)} 0%, ${hex(raw, 0.04)} 120px, transparent 240px)`
          : isEven ? 'transparent' : 'rgba(255,255,255,0.015)',
        transition: 'background 160ms ease',
        paddingLeft: 0,
      }}
    >
      {/* Animated left accent */}
      <div
        style={{
          width: hovered ? '3px' : '2px',
          alignSelf: 'stretch',
          background: hovered
            ? `linear-gradient(to bottom, ${hex(raw, 0.9)}, ${hex(raw, 0.5)})`
            : `linear-gradient(to bottom, ${hex(raw, 0.4)}, ${hex(raw, 0.15)})`,
          borderRadius: '0 2px 2px 0',
          transition: 'all 160ms ease',
          flexShrink: 0,
        }}
      />

      {/* Color dot + label */}
      <div className="flex items-center gap-2 w-14 shrink-0 pl-2">
        <span
          className="w-2 h-2 rounded-full shrink-0"
          style={{
            background: raw,
            boxShadow: hovered ? `0 0 6px ${hex(raw, 0.7)}` : 'none',
            transition: 'box-shadow 160ms ease',
          }}
        />
        <span className="text-[9px] font-mono font-semibold tracking-wide" style={{ color: hex(raw, 0.7) }}>
          {cfg.label}
        </span>
      </div>

      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-text-primary truncate leading-tight">{entry.title || 'Untitled'}</p>
        {showSubtitle && (
          <p className="text-xs text-text-tertiary truncate leading-tight">{getSubtitle(entry) || ' '}</p>
        )}
      </div>

      {showStrength && entry.strength != null && (
        <div className="w-14 shrink-0">
          <div className="h-1 rounded-full overflow-hidden" style={{ background: hex(raw, 0.12) }}>
            <div
              className="h-full rounded-full"
              style={{ width: `${(entry.strength / 5) * 100}%`, background: sColor }}
            />
          </div>
        </div>
      )}

      {showTimestamp && (
        <span className="text-xs text-text-tertiary w-14 text-right shrink-0 tabular-nums">
          {relativeTime(entry.updatedAt)}
        </span>
      )}
    </div>
  )
}

// ── Compact View ──────────────────────────────────────────────────────────────
function CompactCard({ entry, onContextMenu }) {
  const [hovered, setHovered] = useState(false)
  const fetchEntry   = useGrimStore((s) => s.fetchEntry)
  const showTimestamp = useGrimStore((s) => s.preferences.showEntryTimestamp ?? true)
  const cfg = getCfg(entry)
  const raw = cfg.useAccent ? getAccentHex() : cfg.raw

  return (
    <div
      onClick={() => fetchEntry(entry.id)}
      onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); onContextMenu?.(e, entry) }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      className="flex items-center gap-3 h-6 px-4 cursor-pointer border-b border-border/30"
      style={{
        background: hovered ? hex(raw, 0.07) : 'transparent',
        transition: 'background 120ms ease',
      }}
    >
      <span
        className="w-1.5 h-1.5 rounded-full shrink-0"
        style={{
          background: raw,
          boxShadow: hovered ? `0 0 5px ${hex(raw, 0.8)}` : 'none',
          transition: 'box-shadow 120ms ease',
        }}
      />
      <span className="flex-1 min-w-0 text-xs text-text-primary truncate">
        {entry.title || 'Untitled'}
      </span>
      {showTimestamp && (
        <span className="text-[10px] text-text-tertiary w-14 text-right shrink-0 tabular-nums">
          {relativeTime(entry.updatedAt)}
        </span>
      )}
    </div>
  )
}

// ── Public API ────────────────────────────────────────────────────────────────
export function EntryCard({ entry, viewMode = 'grid', listView = false, onContextMenu, index = 0 }) {
  const effectiveMode = listView ? 'list' : viewMode

  if (effectiveMode === 'compact') return <CompactCard entry={entry} onContextMenu={onContextMenu} />
  if (effectiveMode === 'list')    return <ListCard    entry={entry} onContextMenu={onContextMenu} index={index} />
  return                                  <GridCard    entry={entry} onContextMenu={onContextMenu} />
}
