/**
 * Domain-Matcher — Extrahiert Keywords aus Fenstertiteln und matched sie
 * gegen Vault-Einträge, um das richtige Passwort für Strg+G zu finden.
 *
 * Funktioniert für:
 * - Browser-Fenster: "GitHub · Where software... - Google Chrome" → "github.com"
 * - Desktop-Apps:     "FileZilla — Schnelle Verbindung"          → "filezilla"
 * - Terminal:         "alice@vps-01: ~"                          → "vps-01"
 */

const BROWSER_APPS = [
  'chrome', 'chromium', 'firefox', 'edge', 'brave', 'opera',
  'safari', 'arc', 'vivaldi', 'thorium', 'msedge',
]

function isBrowser(appName) {
  if (!appName) return false
  const lower = appName.toLowerCase()
  return BROWSER_APPS.some(b => lower.includes(b))
}

function stripBrowserSuffix(title, appName) {
  // Browser-Titel haben meist das Format "Seitentitel - Browser-Name"
  if (!isBrowser(appName)) return title.trim()

  const lowerApp = (appName || '').toLowerCase()
  for (const sep of [' - ' + appName, ' — ' + appName, ' - ' + lowerApp.replace(/ /g, ''), ' | ' + appName]) {
    const idx = title.lastIndexOf(sep)
    if (idx > 0) return title.substring(0, idx).trim()
  }

  // Fallback: alles nach dem letzten " - " abschneiden
  const parts = title.split(' - ')
  if (parts.length >= 2) return parts.slice(0, -1).join(' - ').trim()

  return title.trim()
}

/**
 * Extrahiert relevante Keywords aus einem Fenstertitel.
 * Gibt ein Array von Suchbegriffen zurück (sortiert nach Relevanz).
 */
export function extractKeywords(windowTitle, appName = '') {
  const cleaned = stripBrowserSuffix(windowTitle, appName)
  if (!cleaned) return []

  const keywords = []
  const lower = cleaned.toLowerCase()

  // 1. Bekannte Sonderfälle
  const patterns = [
    // "Sign in to NAME" / "Login — NAME"
    /sign\s+in\s+(?:to|at|with)\s+([a-zA-Z0-9][-a-zA-Z0-9.]*)/i,
    /login\s*(?:—|—|-|·|to|at)\s*([a-zA-Z0-9][-a-zA-Z0-9.]*)/i,
    // "NAME — Login" / "NAME · Sign in"
    /^([a-zA-Z0-9][-a-zA-Z0-9.]*)\s*(?:—|—|-|·)\s*(?:login|sign\s*in|anmelden)/i,
  ]
  for (const p of patterns) {
    const m = cleaned.match(p)
    if (m?.[1]) keywords.push(m[1].toLowerCase())
  }

  // 2. Wörter die wie Domain-Namen aussehen (enthalten einen Punkt)
  const domainMatch = cleaned.match(/([a-zA-Z0-9][-a-zA-Z0-9.]{2,60}\.[a-zA-Z]{2,20})/g)
  if (domainMatch) {
    for (const d of domainMatch) {
      const base = d.split('.')[0].toLowerCase()
      if (base && base.length > 1 && !['www', 'app', 'api', 'mail', 'login'].includes(base)) {
        keywords.push(base, d.toLowerCase())
      }
    }
  }

  // 3. Allgemeine Keyword-Extraktion — Wörter die mit Großbuchstaben beginnen (Eigennamen)
  const words = cleaned.split(/[\s·—–:]+/).filter(w => w.length > 2)
  for (const w of words) {
    const lowerW = w.toLowerCase()
    if (/^[A-Z][a-z]+$/.test(w) && !keywords.includes(lowerW)) keywords.push(lowerW)
  }

  // 4. Erste signifikante Wörter (falls noch nichts gefunden)
  if (keywords.length === 0) {
    for (const w of words) {
      const lowerW = w.toLowerCase()
      if (lowerW.length > 2 && !['the', 'and', 'for', 'with', 'new', 'tab', 'page', 'window', 'fenster'].includes(lowerW)) {
        keywords.push(lowerW)
      }
    }
    if (keywords.length > 3) keywords.length = 3
  }

  return [...new Set(keywords)]
}

/**
 * Durchsucht die Vault-Einträge nach Matches zu den gegebenen Keywords.
 * Gibt ein nach Relevanz sortiertes Array von Einträgen zurück.
 */
export function matchEntries(entries, keywords) {
  if (!keywords.length || !entries.length) return []

  const scored = entries
    .filter(e => {
      // Nur PASSWORD-Einträge durchsuchen (oder Einträge mit url-Feld)
      const cat = (e.category || '').toUpperCase()
      if (cat && cat !== 'PASSWORD' && cat !== 'CERTIFICATE') return false
      return true
    })
    .map(e => {
      let score = 0
      const title = (e.title || '').toLowerCase()
      const url = (e.fields?.url || e.url || '').toLowerCase()
      const username = (e.fields?.username || e.username || '').toLowerCase()
      const domain = (e.fields?.domain || e.entry_domain || '').toLowerCase()

      for (const kw of keywords) {
        // Exakte URL-Domain-Match (höchste Priorität)
        if (url === kw || domain === kw) score += 100
        // URL enthält Keyword
        else if (url.includes(kw) || domain.includes(kw)) score += 50
        // Title-Match
        else if (title.includes(kw)) score += 30
        // Keyword ist Teil des Titels (partiell)
        else if (kw.includes(title) || title.split(/[\s\-_.]+/).some(p => kw.includes(p))) score += 15
        // Username enthält Keyword
        if (username.includes(kw)) score += 5
      }

      return { entry: e, score }
    })
    .filter(s => s.score > 0)
    .sort((a, b) => b.score - a.score)

  return scored.map(s => s.entry)
}
