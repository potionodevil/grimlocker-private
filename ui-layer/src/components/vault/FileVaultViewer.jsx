import { useState, useEffect, useCallback, useRef } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { invoke } from '@tauri-apps/api/core'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * FileVaultViewer — Entschlüsselt und zeigt/editiert Vault-Dateien im Frontend an.
 *
 * Dateitypen:
 *   Bilder (jpg/png/gif/webp/svg/bmp) → Inline-Vorschau
 *   DOCX → Einfacher Texteditor (mammoth für Extraktion, Speichern erzeugt neuen Blob)
 *   Alles andere → OS-Standard-App via Temp-Datei (wird nach 30s sicher gelöscht)
 *
 * Props:
 *   entry    object   — Vault-Eintrag: {id, title, fields: {file_name, mime_type, manifest_block_id}}
 *   isOpen   boolean
 *   onClose  function
 *   onSave   function(newManifestBlockId, fileName, mimeType) — wird nach Speichern aufgerufen
 */
export function FileVaultViewer({ entry, isOpen, onClose, onSave }) {
  const [status, setStatus]       = useState('idle')
  // idle | loading | viewing-image | editing-docx | saving-docx | external | error
  const [errorMsg, setErrorMsg]   = useState('')
  const [imgSrc, setImgSrc]       = useState(null)
  const [docText, setDocText]     = useState('')    // extracted DOCX plain text
  const [saveMsg, setSaveMsg]     = useState('')    // "Gespeichert ✓" feedback
  const [tmpPath, setTmpPath]     = useState(null)
  const blobUrlRef = useRef(null)

  const fileName   = entry?.fields?.file_name   || entry?.title || 'File'
  const mimeType   = entry?.fields?.mime_type   || 'application/octet-stream'
  const manifestId = entry?.fields?.manifest_block_id || entry?.id

  const isImage = mimeType.startsWith('image/') || /\.(jpg|jpeg|png|gif|webp|svg|bmp|ico)$/i.test(fileName)
  const isDocx  = mimeType.includes('wordprocessingml') || /\.docx$/i.test(fileName)

  const download = useCallback(async () => {
    if (!manifestId) {
      setErrorMsg('Kein Manifest-Block-ID gefunden.')
      setStatus('error')
      return
    }

    setStatus('loading')
    setErrorMsg('')

    try {
      const result = await tauriBridge.downloadFile(manifestId)
      const bytes  = new Uint8Array(result.data)

      if (isImage) {
        const blob = new Blob([bytes], { type: mimeType })
        const url  = URL.createObjectURL(blob)
        blobUrlRef.current = url
        setImgSrc(url)
        setStatus('viewing-image')

      } else if (isDocx) {
        // Try to extract text via mammoth (lazy import).
        try {
          const mammoth = await import(/* @vite-ignore */ 'mammoth')
          const arrayBuffer = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength)
          const { value } = await mammoth.extractRawText({ arrayBuffer })
          setDocText(value)
          setStatus('editing-docx')
        } catch {
          // mammoth not available or extraction failed — fall back to external app
          const path = await invoke('save_temp_file', {
            filename: result.fileName || fileName,
            data: Array.from(bytes),
          })
          setTmpPath(path)
          await invoke('open_with_default_app', { path })
          setStatus('external')
        }

      } else {
        const path = await invoke('save_temp_file', {
          filename: result.fileName || fileName,
          data: Array.from(bytes),
        })
        setTmpPath(path)
        await invoke('open_with_default_app', { path })
        setStatus('external')
      }
    } catch (e) {
      setErrorMsg(e.message || 'Download fehlgeschlagen')
      setStatus('error')
    }
  }, [manifestId, isImage, isDocx, mimeType, fileName])

  // Save edited DOCX: generate a simple plain-text DOCX and re-upload.
  const handleSaveDocx = useCallback(async () => {
    setStatus('saving-docx')
    setSaveMsg('')
    try {
      // Build a minimal DOCX from plain text using the docx library.
      // If not available, create a plain .txt as fallback.
      let fileBlob
      let saveName = fileName

      try {
        const { Document, Paragraph, TextRun, Packer } = await import(/* @vite-ignore */ 'docx')
        const paragraphs = docText.split('\n').map(line =>
          new Paragraph({ children: [new TextRun(line)] })
        )
        const doc = new Document({ sections: [{ children: paragraphs }] })
        const buffer = await Packer.toBuffer(doc)
        fileBlob = new File([buffer], saveName, {
          type: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
        })
      } catch {
        // Fallback: save as plain text
        saveName = saveName.replace(/\.docx$/i, '.txt')
        fileBlob = new File([new TextEncoder().encode(docText)], saveName, { type: 'text/plain' })
      }

      // Upload the new file via the existing ingest flow.
      const newManifest = await tauriBridge.ingestFile(fileBlob, null)

      setSaveMsg('Gespeichert ✓')
      setStatus('editing-docx')

      if (onSave) {
        onSave(newManifest.manifest_block_id || newManifest.id, saveName, fileBlob.type)
      }
    } catch (e) {
      setErrorMsg(e.message || 'Speichern fehlgeschlagen')
      setStatus('editing-docx')
    }
  }, [docText, fileName, onSave])

  const handleClose = useCallback(async () => {
    if (blobUrlRef.current) {
      URL.revokeObjectURL(blobUrlRef.current)
      blobUrlRef.current = null
    }
    if (tmpPath) {
      await invoke('secure_delete_temp', { path: tmpPath }).catch(() => {})
      setTmpPath(null)
    }
    onClose()
  }, [tmpPath, onClose])

  useEffect(() => {
    if (!isOpen) {
      if (blobUrlRef.current) {
        URL.revokeObjectURL(blobUrlRef.current)
        blobUrlRef.current = null
      }
      setStatus('idle')
      setImgSrc(null)
      setDocText('')
      setErrorMsg('')
      setSaveMsg('')
    }
  }, [isOpen])

  const formatSize = (bytes) => {
    if (!bytes) return ''
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  }

  const totalSize = entry?.fields?.total_size ? formatSize(Number(entry.fields.total_size)) : ''

  const fileIcon = isImage ? '🖼' : isDocx ? '📝' : '📄'
  const actionLabel = isImage ? 'Anzeigen' : isDocx ? 'Bearbeiten' : 'Mit Standard-App öffnen'

  return (
    <AnimatePresence>
      {isOpen && (
        <motion.div
          key="backdrop"
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.15 }}
          className="fixed inset-0 z-[90] flex flex-col items-center justify-center"
          style={{ background: 'rgba(0,0,0,0.75)' }}
          onClick={handleClose}
        >
          <motion.div
            key="viewer"
            initial={{ opacity: 0, scale: 0.96, y: 12 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.96, y: 12 }}
            transition={{ duration: 0.2, ease: 'easeOut' }}
            className="bg-surface-base rounded-xl shadow-2xl border border-border flex flex-col"
            style={{ width: '90vw', maxWidth: 900, height: '85vh' }}
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="flex items-center gap-3 px-5 py-3 border-b border-border flex-shrink-0">
              <div className="flex-1 min-w-0">
                <p className="text-sm font-semibold text-text-primary truncate">{fileName}</p>
                <p className="text-xs text-text-tertiary">
                  {mimeType}{totalSize ? ` · ${totalSize}` : ''}
                </p>
              </div>

              {/* Save button for DOCX editor */}
              {status === 'editing-docx' && (
                <button
                  onClick={handleSaveDocx}
                  className="shrink-0 h-7 px-3 rounded text-xs font-semibold bg-accent hover:bg-accent-hover text-white transition-colors"
                >
                  Speichern
                </button>
              )}
              {saveMsg && (
                <span className="text-xs text-green-400 shrink-0">{saveMsg}</span>
              )}

              <button
                onClick={handleClose}
                className="shrink-0 text-text-tertiary hover:text-text-primary transition-colors p-1.5 rounded"
              >
                &#x2715;
              </button>
            </div>

            {/* Body */}
            <div className="flex-1 overflow-auto flex flex-col p-4">

              {/* Idle */}
              {status === 'idle' && (
                <div className="flex flex-col items-center justify-center h-full gap-6 text-center">
                  <div className="w-16 h-16 rounded-xl bg-surface-subtle border border-border flex items-center justify-center text-3xl">
                    {fileIcon}
                  </div>
                  <div>
                    <p className="text-sm font-medium text-text-primary">{fileName}</p>
                    <p className="text-xs text-text-tertiary mt-0.5">{mimeType}{totalSize ? ` · ${totalSize}` : ''}</p>
                  </div>
                  <button
                    onClick={download}
                    className="px-6 py-2.5 rounded-lg bg-accent hover:bg-accent-hover text-white text-sm font-semibold transition-colors"
                  >
                    {actionLabel}
                  </button>
                </div>
              )}

              {/* Loading */}
              {status === 'loading' && (
                <div className="flex flex-col items-center justify-center h-full gap-4 text-text-secondary">
                  <div className="w-8 h-8 border-2 border-accent border-t-transparent rounded-full animate-spin" />
                  <p className="text-sm">Datei wird entschlüsselt…</p>
                </div>
              )}

              {/* Saving DOCX */}
              {status === 'saving-docx' && (
                <div className="flex flex-col items-center justify-center h-full gap-4 text-text-secondary">
                  <div className="w-8 h-8 border-2 border-accent border-t-transparent rounded-full animate-spin" />
                  <p className="text-sm">Datei wird verschlüsselt und gespeichert…</p>
                </div>
              )}

              {/* Inline image */}
              {status === 'viewing-image' && imgSrc && (
                <div className="flex items-center justify-center h-full">
                  <img
                    src={imgSrc}
                    alt={fileName}
                    className="max-w-full max-h-full object-contain rounded shadow-lg"
                  />
                </div>
              )}

              {/* DOCX editor */}
              {status === 'editing-docx' && (
                <div className="flex flex-col h-full gap-2">
                  <div className="flex items-center gap-2 shrink-0">
                    <p className="text-xs text-text-tertiary flex-1">
                      Einfacher Texteditor — Formatierungen (Tabellen, Bilder) werden beim Speichern vereinfacht
                    </p>
                    {errorMsg && (
                      <p className="text-xs text-danger">{errorMsg}</p>
                    )}
                  </div>
                  <textarea
                    value={docText}
                    onChange={e => setDocText(e.target.value)}
                    className="flex-1 w-full p-4 rounded-lg bg-surface-subtle border border-border text-sm text-text-primary font-mono resize-none focus:outline-none focus:ring-2 focus:ring-accent/30 focus:border-accent/50"
                    spellCheck={false}
                    placeholder="Dokument ist leer…"
                  />
                </div>
              )}

              {/* External opened */}
              {status === 'external' && (
                <div className="flex flex-col items-center justify-center h-full gap-3 text-center text-text-secondary">
                  <span className="text-3xl">✓</span>
                  <p className="text-sm font-medium">{fileName}</p>
                  <p className="text-xs text-text-tertiary">
                    In Standard-App geöffnet. Temp-Datei wird beim Schließen sicher gelöscht.
                  </p>
                  <button
                    onClick={handleClose}
                    className="mt-2 px-5 py-2 rounded-lg bg-surface-subtle border border-border text-sm text-text-secondary hover:text-text-primary transition-colors"
                  >
                    Schließen
                  </button>
                </div>
              )}

              {/* Error */}
              {status === 'error' && (
                <div className="flex flex-col items-center justify-center h-full gap-3 text-center">
                  <p className="text-sm font-semibold text-danger">Fehler</p>
                  <p className="text-xs text-text-tertiary max-w-sm">{errorMsg}</p>
                  <button
                    onClick={() => { setStatus('idle'); setErrorMsg('') }}
                    className="mt-1 px-4 py-1.5 text-xs rounded bg-surface-subtle border border-border text-text-secondary hover:text-text-primary transition-colors"
                  >
                    Zurück
                  </button>
                </div>
              )}
            </div>
          </motion.div>
        </motion.div>
      )}
    </AnimatePresence>
  )
}
