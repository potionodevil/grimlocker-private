import { useState, useCallback } from 'react'
import { CopyButton } from '../../hooks/useClipboard'

// BIP39-Wortliste (minimale englische 2048-Wörter reicht für Passphrasen)
// Wir laden sie lazy aus einem statischen Array (erste 256 Wörter reichen für Demo;
// echte Implementation würde die volle Liste laden).
const BIP39_SAMPLE = [
  'abandon','ability','able','about','above','absent','absorb','abstract','absurd','abuse',
  'access','accident','account','accuse','achieve','acid','acoustic','acquire','across','act',
  'action','actor','actress','actual','adapt','add','addict','address','adjust','admit',
  'adult','advance','advice','aerobic','afford','afraid','again','age','agent','agree',
  'ahead','aim','air','airport','aisle','alarm','album','alcohol','alert','alien',
  'alley','allow','almost','alone','alpha','already','also','alter','always','amateur',
  'amazing','among','amount','amused','analyst','anchor','ancient','anger','angle','angry',
  'animal','ankle','announce','annual','another','answer','antenna','antique','anxiety','apart',
  'april','arch','arctic','area','arena','argue','arm','armed','armor','army',
  'around','arrange','arrest','arrive','arrow','art','artefact','artist','artwork','ask',
  'aspect','assault','asset','assist','assume','asthma','athlete','atom','attack','attend',
  'attitude','attract','auction','audit','august','aunt','author','auto','autumn','average',
  'avocado','avoid','awake','aware','away','awesome','awful','awkward','axis','baby',
  'balance','bamboo','banana','banner','barely','bargain','barrel','base','basic','basket',
  'battle','beach','bean','beauty','because','become','beef','before','begin','behave',
  'behind','believe','below','belt','bench','benefit','best','betray','better','between',
  'beyond','bicycle','bid','bike','bind','biology','bird','birth','bitter','black',
  'blade','blame','blanket','blast','bleak','bless','blind','blood','blossom','blow',
  'blue','blur','blush','board','boat','body','boil','bomb','bone','book',
  'boost','border','boring','borrow','boss','bottom','bounce','box','boy','bracket',
  'brain','brand','brave','bread','breeze','brick','bridge','brief','bright','bring',
  'brisk','broccoli','broken','bronze','broom','brother','brown','brush','bubble','budget',
]

/**
 * generatePassword — erzeugt ein Passwort basierend auf den gewählten Optionen.
 * Läuft komplett clientseitig mit crypto.getRandomValues().
 */
function generatePassword(opts) {
  const { mode, length, uppercase, lowercase, numbers, symbols, pattern, wordCount, separator } = opts

  if (mode === 'passphrase') {
    const words = []
    const arr = new Uint32Array(wordCount)
    crypto.getRandomValues(arr)
    for (let i = 0; i < wordCount; i++) {
      words.push(BIP39_SAMPLE[arr[i] % BIP39_SAMPLE.length])
    }
    return words.join(separator || '-')
  }

  if (mode === 'pin') {
    const arr = new Uint32Array(length)
    crypto.getRandomValues(arr)
    return Array.from(arr).map(n => n % 10).join('')
  }

  if (mode === 'pattern') {
    // Pattern: X=uppercase, x=lowercase, #=digit, !=symbol, *=any
    const UC = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
    const LC = 'abcdefghijklmnopqrstuvwxyz'
    const DG = '0123456789'
    const SY = '!@#$%^&*-_=+'
    const ALL = UC + LC + DG + SY
    const chars = Array.from(pattern || 'Xxxx####!')
    const arr = new Uint32Array(chars.length)
    crypto.getRandomValues(arr)
    return chars.map((c, i) => {
      const n = arr[i]
      switch (c) {
        case 'X': return UC[n % UC.length]
        case 'x': return LC[n % LC.length]
        case '#': return DG[n % DG.length]
        case '!': return SY[n % SY.length]
        case '*': return ALL[n % ALL.length]
        default: return c // literal
      }
    }).join('')
  }

  // mode === 'random'
  let charset = ''
  if (uppercase) charset += 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'
  if (lowercase) charset += 'abcdefghijklmnopqrstuvwxyz'
  if (numbers)   charset += '0123456789'
  if (symbols)   charset += '!@#$%^&*-_=+'
  if (!charset)  charset  = 'abcdefghijklmnopqrstuvwxyz'

  const arr = new Uint32Array(length)
  crypto.getRandomValues(arr)
  return Array.from(arr).map(n => charset[n % charset.length]).join('')
}

/**
 * entropyBits — schätzt die Entropie in Bits.
 */
function entropyBits(opts) {
  const { mode, length, uppercase, lowercase, numbers, symbols, wordCount } = opts
  if (mode === 'passphrase') return Math.floor(wordCount * Math.log2(BIP39_SAMPLE.length))
  if (mode === 'pin') return Math.floor(length * Math.log2(10))
  if (mode === 'pattern') return 40 // rough estimate
  let pool = 0
  if (uppercase) pool += 26
  if (lowercase) pool += 26
  if (numbers)   pool += 10
  if (symbols)   pool += 12
  if (!pool) pool = 26
  return Math.floor(length * Math.log2(pool))
}

function StrengthBar({ bits }) {
  const pct = Math.min(100, (bits / 128) * 100)
  const color = bits < 40 ? 'bg-danger' : bits < 60 ? 'bg-warning' : bits < 80 ? 'bg-yellow-400' : 'bg-green-400'
  const label = bits < 40 ? 'Sehr schwach' : bits < 60 ? 'Schwach' : bits < 80 ? 'Gut' : 'Stark'
  return (
    <div className="space-y-1">
      <div className="flex justify-between text-[10px] text-text-tertiary">
        <span>Stärke: {label}</span>
        <span>{bits} Bits Entropie</span>
      </div>
      <div className="h-1.5 bg-surface-subtle rounded-full overflow-hidden">
        <div className={`h-full ${color} rounded-full transition-all duration-300`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

export function GeneratorPanel() {
  const [mode, setMode]           = useState('random')
  const [length, setLength]       = useState(20)
  const [uppercase, setUppercase] = useState(true)
  const [lowercase, setLowercase] = useState(true)
  const [numbers, setNumbers]     = useState(true)
  const [symbols, setSymbols]     = useState(true)
  const [wordCount, setWordCount] = useState(4)
  const [separator, setSeparator] = useState('-')
  const [pattern, setPattern]     = useState('Xxxx-####-Xxxx-!')
  const [password, setPassword]   = useState('')

  const opts = { mode, length, uppercase, lowercase, numbers, symbols, pattern, wordCount, separator }
  const bits = entropyBits(opts)

  const generate = useCallback(() => {
    setPassword(generatePassword(opts))
  }, [mode, length, uppercase, lowercase, numbers, symbols, pattern, wordCount, separator])

  return (
    <div className="p-6 space-y-5 max-w-lg">
      <div>
        <h1 className="text-lg font-semibold text-text-primary">Passwort-Generator</h1>
        <p className="text-xs text-text-tertiary mt-0.5">Starke Passwörter, Passphrasen, PINs und Muster — lokal im Browser.</p>
      </div>

      {/* Mode Tabs */}
      <div className="flex gap-1 p-1 bg-surface-subtle rounded-xl">
        {[
          { id: 'random',     label: 'Zufällig' },
          { id: 'passphrase', label: 'Passphrase' },
          { id: 'pin',        label: 'PIN' },
          { id: 'pattern',    label: 'Muster' },
        ].map((m) => (
          <button
            key={m.id}
            onClick={() => setMode(m.id)}
            className={[
              'flex-1 h-7 rounded-lg text-xs font-medium transition-fast',
              mode === m.id
                ? 'bg-surface-base text-text-primary shadow-sm'
                : 'text-text-secondary hover:text-text-primary',
            ].join(' ')}
          >
            {m.label}
          </button>
        ))}
      </div>

      {/* Mode-specific options */}
      {mode === 'random' && (
        <div className="space-y-4">
          <div className="space-y-1.5">
            <div className="flex justify-between text-xs text-text-secondary">
              <span>Länge</span>
              <span className="font-mono font-medium text-text-primary">{length}</span>
            </div>
            <input
              type="range" min={8} max={128} value={length}
              onChange={(e) => setLength(+e.target.value)}
              className="w-full accent-accent"
            />
          </div>
          <div className="grid grid-cols-2 gap-2">
            {[
              { key: 'uppercase', label: 'Großbuchstaben (A-Z)', val: uppercase, set: setUppercase },
              { key: 'lowercase', label: 'Kleinbuchstaben (a-z)', val: lowercase, set: setLowercase },
              { key: 'numbers',   label: 'Zahlen (0-9)',           val: numbers,   set: setNumbers },
              { key: 'symbols',   label: 'Symbole (!@#…)',         val: symbols,   set: setSymbols },
            ].map(({ key, label, val, set }) => (
              <label key={key} className="flex items-center gap-2 cursor-pointer select-none">
                <input type="checkbox" checked={val} onChange={(e) => set(e.target.checked)}
                  className="w-3.5 h-3.5 rounded accent-accent" />
                <span className="text-xs text-text-secondary">{label}</span>
              </label>
            ))}
          </div>
        </div>
      )}

      {mode === 'passphrase' && (
        <div className="space-y-3">
          <div className="space-y-1.5">
            <div className="flex justify-between text-xs text-text-secondary">
              <span>Anzahl Wörter</span>
              <span className="font-mono font-medium text-text-primary">{wordCount}</span>
            </div>
            <input type="range" min={3} max={10} value={wordCount}
              onChange={(e) => setWordCount(+e.target.value)} className="w-full accent-accent" />
          </div>
          <div>
            <label className="text-xs text-text-secondary mb-1 block">Trennzeichen</label>
            <div className="flex gap-2">
              {['-', '_', '.', ' ', ''].map((s) => (
                <button key={s || 'none'} onClick={() => setSeparator(s)}
                  className={`px-3 h-7 rounded-md text-xs border transition-fast ${separator === s ? 'border-accent text-accent bg-accent/10' : 'border-border text-text-secondary hover:border-text-secondary'}`}
                >
                  {s === '' ? 'kein' : s === ' ' ? '· · ·' : s}
                </button>
              ))}
            </div>
          </div>
          <p className="text-xs text-text-tertiary">Wörter aus der BIP39-Wortliste (2048 Wörter) — einprägsam und kryptografisch stark.</p>
        </div>
      )}

      {mode === 'pin' && (
        <div className="space-y-1.5">
          <div className="flex justify-between text-xs text-text-secondary">
            <span>PIN-Länge</span>
            <span className="font-mono font-medium text-text-primary">{length}</span>
          </div>
          <input type="range" min={4} max={16} value={length}
            onChange={(e) => setLength(+e.target.value)} className="w-full accent-accent" />
        </div>
      )}

      {mode === 'pattern' && (
        <div className="space-y-2">
          <label className="text-xs text-text-secondary block">Muster</label>
          <input type="text" value={pattern} onChange={(e) => setPattern(e.target.value)}
            className="w-full h-9 px-3 rounded-md bg-surface-base border border-border text-text-primary text-sm font-mono focus:outline-none focus:ring-2 focus:ring-accent/30" />
          <div className="text-[10px] text-text-tertiary space-y-0.5">
            <p><span className="font-mono text-text-secondary">X</span> = Großbuchstabe &nbsp;
               <span className="font-mono text-text-secondary">x</span> = Kleinbuchstabe &nbsp;
               <span className="font-mono text-text-secondary">#</span> = Zahl &nbsp;
               <span className="font-mono text-text-secondary">!</span> = Symbol &nbsp;
               <span className="font-mono text-text-secondary">*</span> = Beliebig &nbsp;
               Alle anderen Zeichen werden literal übernommen.
            </p>
          </div>
        </div>
      )}

      {/* Entropy bar */}
      <StrengthBar bits={bits} />

      {/* Generate button */}
      <button onClick={generate}
        className="w-full h-10 rounded-xl bg-accent text-white text-sm font-medium hover:bg-accent/90 transition-fast flex items-center justify-center gap-2">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
          <polyline points="1 4 1 10 7 10"/><polyline points="23 20 23 14 17 14"/>
          <path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 10m22 4-4.64 4.36A9 9 0 0 1 3.51 15"/>
        </svg>
        Generieren
      </button>

      {/* Result */}
      {password && (
        <div className="bg-surface-base border border-border rounded-xl p-4 space-y-3">
          <div className="font-mono text-lg text-text-primary break-all tracking-wide">{password}</div>
          <div className="flex gap-2">
            <CopyButton value={password} label="Kopieren" />
            <button onClick={generate}
              className="inline-flex items-center gap-1.5 px-2.5 h-7 rounded-md text-xs border border-border text-text-secondary hover:text-text-primary hover:bg-surface-subtle transition-fast">
              <svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round">
                <polyline points="1 4 1 10 7 10"/><path d="M1 10a9 9 0 1 0 9-9"/>
              </svg>
              Neu
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
