import { useState, useRef, useCallback } from 'react'
import { tauriBridge } from '../../services/tauriBridge'

/**
 * FileVaultUpload — drag-and-drop / browse file ingestion for the FileVault tab.
 *
 * Protocol: MSG_FILE_INGEST_BEGIN (0x20) → MSG_FILE_CHUNK (0x21)× N → MSG_FILE_INGEST_END (0x22)
 * Uses the existing backend streaming protocol; no new backend code required.
 *
 * Props:
 *   onSuccess(manifest) — called when the daemon confirms the upload.
 *   onCancel()          — called when the user dismisses without uploading.
 */
export function FileVaultUpload({ onSuccess, onCancel }) {
  const [dragging, setDragging]     = useState(false)
  const [file, setFile]             = useState(null)
  const [progress, setProgress]     = useState(0)     // 0..1
  const [uploading, setUploading]   = useState(false)
  const [error, setError]           = useState(null)
  const fileInputRef                = useRef(null)

  const handleDragEnter = useCallback((e) => {
    e.preventDefault()
    e.stopPropagation()
    setDragging(true)
  }, [])

  const handleDragLeave = useCallback((e) => {
    e.preventDefault()
    e.stopPropagation()
    setDragging(false)
  }, [])

  const handleDrop = useCallback((e) => {
    e.preventDefault()
    e.stopPropagation()
    setDragging(false)
    const dropped = e.dataTransfer.files[0]
    if (dropped) {
      setFile(dropped)
      setError(null)
      setProgress(0)
    }
  }, [])

  const handleBrowse = useCallback((e) => {
    const picked = e.target.files[0]
    if (picked) {
      setFile(picked)
      setError(null)
      setProgress(0)
    }
  }, [])

  const handleUpload = useCallback(async () => {
    if (!file || uploading) return

    setUploading(true)
    setError(null)
    setProgress(0)

    try {
      const manifest = await tauriBridge.ingestFile(file, (pct) => {
        setProgress(pct)
      })
      setProgress(1)
      onSuccess?.(manifest)
    } catch (err) {
      console.error('[FileVaultUpload] Ingest failed:', err)
      setError(err.message || 'Upload failed')
    } finally {
      setUploading(false)
    }
  }, [file, uploading, onSuccess])

  const handleClear = useCallback(() => {
    setFile(null)
    setProgress(0)
    setError(null)
    if (fileInputRef.current) fileInputRef.current.value = ''
  }, [])

  const formatSize = (bytes) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / 1024 / 1024).toFixed(2)} MB`
  }

  return (
    <div className="space-y-3">
      {/* Drop zone */}
      <div
        onDragEnter={handleDragEnter}
        onDragOver={handleDragEnter}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => !file && fileInputRef.current?.click()}
        className={[
          'border-2 border-dashed rounded-lg p-8 text-center cursor-pointer transition-fast',
          dragging
            ? 'border-accent bg-accent/5 text-accent'
            : file
              ? 'border-border bg-surface-base cursor-default'
              : 'border-border hover:border-accent/50 hover:bg-surface-subtle text-text-secondary',
        ].join(' ')}
      >
        {file ? (
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3 text-left">
              <span className="text-2xl">📄</span>
              <div>
                <p className="text-sm font-medium text-text-primary">{file.name}</p>
                <p className="text-xs text-text-secondary">{formatSize(file.size)}</p>
              </div>
            </div>
            {!uploading && (
              <button
                onClick={(e) => { e.stopPropagation(); handleClear() }}
                className="w-6 h-6 flex items-center justify-center rounded text-text-tertiary hover:text-text-primary hover:bg-surface-subtle transition-fast text-sm"
                title="Remove file"
              >
                ✕
              </button>
            )}
          </div>
        ) : (
          <>
            <div className="text-3xl mb-2">📂</div>
            <p className="text-sm font-medium mb-1">Drop a file here</p>
            <p className="text-xs text-text-tertiary">or click to browse</p>
          </>
        )}
      </div>

      {/* Hidden file input */}
      <input
        ref={fileInputRef}
        type="file"
        className="hidden"
        onChange={handleBrowse}
      />

      {/* Progress bar */}
      {uploading && (
        <div className="space-y-1">
          <div className="flex items-center justify-between text-xs text-text-secondary">
            <span>Encrypting & uploading…</span>
            <span>{Math.round(progress * 100)}%</span>
          </div>
          <div className="w-full h-1.5 bg-surface-subtle rounded-full overflow-hidden">
            <div
              className="h-full bg-accent rounded-full transition-all duration-200"
              style={{ width: `${Math.round(progress * 100)}%` }}
            />
          </div>
        </div>
      )}

      {/* Error */}
      {error && (
        <p className="text-xs text-danger bg-danger/5 border border-danger/20 rounded px-3 py-2">
          {error}
        </p>
      )}

      {/* Action buttons */}
      <div className="flex justify-end gap-2">
        {onCancel && (
          <button
            onClick={onCancel}
            disabled={uploading}
            className="h-8 px-3 rounded-md text-sm text-text-secondary hover:bg-surface-subtle transition-fast disabled:opacity-40"
          >
            Cancel
          </button>
        )}
        <button
          onClick={handleUpload}
          disabled={!file || uploading}
          className="h-8 px-4 rounded-md text-sm font-medium text-white bg-accent hover:bg-accent-hover disabled:opacity-40 disabled:cursor-not-allowed transition-fast"
        >
          {uploading ? 'Uploading…' : '🔒 Encrypt & Store'}
        </button>
      </div>
    </div>
  )
}
