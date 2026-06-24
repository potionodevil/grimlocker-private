import { useState, useMemo } from 'react'

/**
 * NoteView — zeigt den Inhalt einer Sicheren Notiz an.
 * Rendert Markdown mit einem minimalen eingebauten Renderer (kein externes Paket nötig).
 * Unterstützt: Überschriften, Fett/Kursiv, Listen, Code-Blöcke, Links.
 */
export function NoteView({ content = '' }) {
  const html = useMemo(() => renderMarkdown(content), [content])

  return (
    <div
      className="prose prose-sm prose-invert max-w-none text-text-primary"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}

/**
 * NoteEditor — editierbares Textarea + Live-Preview-Tab.
 */
export function NoteEditor({ value, onChange, placeholder = 'Notiz in Markdown schreiben…', rows = 12 }) {
  const [tab, setTab] = useState('edit')
  const html = useMemo(() => renderMarkdown(value), [value])

  return (
    <div className="space-y-2">
      {/* Tab-Bar */}
      <div className="flex gap-1 p-1 bg-surface-subtle rounded-lg w-fit">
        {['edit', 'preview'].map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={[
              'px-3 h-6 rounded-md text-xs transition-fast',
              tab === t
                ? 'bg-surface-base text-text-primary font-medium shadow-sm'
                : 'text-text-secondary hover:text-text-primary',
            ].join(' ')}
          >
            {t === 'edit' ? 'Bearbeiten' : 'Vorschau'}
          </button>
        ))}
      </div>

      {tab === 'edit' ? (
        <textarea
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={rows}
          placeholder={placeholder}
          className="w-full px-3 py-2 rounded-md bg-surface-base border border-border text-text-primary text-sm placeholder:text-text-disabled focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent transition-fast resize-none font-mono"
        />
      ) : (
        <div className="min-h-32 px-3 py-2 rounded-md bg-surface-base border border-border">
          {value.trim()
            ? <NoteView content={value} />
            : <p className="text-text-disabled text-xs italic">Noch kein Inhalt.</p>
          }
        </div>
      )}
    </div>
  )
}

// ─── Minimaler Markdown-Renderer ──────────────────────────────────────────────
// Kein externes Paket. Sicher: kein innerHTML von User-Input ohne Escaping.

function escapeHtml(text) {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function renderMarkdown(md) {
  if (!md) return ''
  const lines = md.split('\n')
  const out = []
  let inCode = false
  let inList = false

  for (let i = 0; i < lines.length; i++) {
    let line = lines[i]

    // Code block
    if (line.startsWith('```')) {
      if (inCode) {
        out.push('</code></pre>')
        inCode = false
      } else {
        if (inList) { out.push('</ul>'); inList = false }
        out.push('<pre class="bg-surface-app rounded-md p-3 text-xs overflow-x-auto my-2"><code>')
        inCode = true
      }
      continue
    }
    if (inCode) {
      out.push(escapeHtml(line))
      continue
    }

    // Close list if needed
    if (inList && !line.match(/^[-*+]\s/)) {
      out.push('</ul>')
      inList = false
    }

    // Headings
    const h3 = line.match(/^###\s+(.+)/)
    const h2 = line.match(/^##\s+(.+)/)
    const h1 = line.match(/^#\s+(.+)/)
    if (h1) { out.push(`<h1 class="text-lg font-bold mt-4 mb-1">${inlineMarkdown(h1[1])}</h1>`); continue }
    if (h2) { out.push(`<h2 class="text-base font-semibold mt-3 mb-1">${inlineMarkdown(h2[1])}</h2>`); continue }
    if (h3) { out.push(`<h3 class="text-sm font-semibold mt-2 mb-0.5">${inlineMarkdown(h3[1])}</h3>`); continue }

    // Horizontal rule
    if (line.match(/^[-*_]{3,}$/)) { out.push('<hr class="border-border my-3" />'); continue }

    // Unordered list
    const li = line.match(/^[-*+]\s+(.+)/)
    if (li) {
      if (!inList) { out.push('<ul class="list-disc list-inside space-y-0.5 text-sm my-1">'); inList = true }
      out.push(`<li>${inlineMarkdown(li[1])}</li>`)
      continue
    }

    // Empty line → paragraph break
    if (line.trim() === '') {
      out.push('<br />')
      continue
    }

    // Regular paragraph
    out.push(`<p class="text-sm leading-relaxed">${inlineMarkdown(escapeHtml(line))}</p>`)
  }

  if (inList) out.push('</ul>')
  if (inCode) out.push('</code></pre>')

  return out.join('\n')
}

function inlineMarkdown(text) {
  return text
    .replace(/`([^`]+)`/g, '<code class="bg-surface-app px-1 rounded text-xs font-mono">$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" class="text-accent underline" target="_blank" rel="noopener noreferrer">$1</a>')
}
