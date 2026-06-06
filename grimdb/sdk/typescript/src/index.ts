/**
 * @grimlocker/sdk — TypeScript/JavaScript SDK for the Grimlocker daemon HTTP API.
 *
 * Wraps the `/api/v1` JSON endpoint exposed by the daemon on localhost.
 * Works in Node.js, browsers, and any environment with the Fetch API.
 *
 * Quick start:
 *   const client = new GrimlockerClient('http://127.0.0.1:PORT', 'YOUR_TOKEN')
 *   await client.unlockVault('master-password')
 *   const entries = await client.listEntries()
 */

// ── Types ─────────────────────────────────────────────────────────────────────

export type EntryCategory = 'PASSWORD' | 'SSH_KEY' | 'CERTIFICATE' | 'FILE_VAULT'

export interface VaultEntry {
  id:         string
  title:      string
  category:   EntryCategory
  created_at: number   // Unix nanoseconds
  updated_at: number
  fields?:    Record<string, string>
}

export interface CertificateEntry extends VaultEntry {
  domain: string
  certificate: string
}

export interface SyncPeer {
  device_id:   string
  host:        string
  port:        number
  seen_at:     number   // Unix nanoseconds
  version?:    Record<string, { v: number; t: number }>
  reachable?:  boolean
}

export interface SyncStatus {
  peers:        SyncPeer[]
  last_sync_at: number   // Unix milliseconds (0 = never)
  device_id:    string
}

export interface AuditEvent {
  timestamp:  number   // Unix nanoseconds
  level:      'INFO' | 'WARN' | 'CRITICAL'
  module:     string
  message:    string
  subject_id?: string
  prev_hash?:  string   // hex
  hash?:       string   // hex
}

export interface FileEntry {
  id:                string
  file_name:         string
  mime_type:         string
  total_size:        number
  manifest_block_id: string
  folder_id:         string
}

export interface FolderItem {
  id:   string
  name: string
  type: 'folder' | 'file'
}

export interface FolderListing {
  folders: FolderItem[]
  files:   FileEntry[]
}

export interface UploadProgress {
  bytes_sent:  number
  total_bytes: number
}

export interface Workspace {
  id:         string
  name:       string
  is_default: boolean
}

export interface HealthStatus {
  status:           string
  daemon_version?:  string
  vault_initialized: boolean
  vault_unlocked:    boolean
}

export interface GrimlockerError {
  message:     string
  action?:     string
  status_code: number
}

export type GrimlockerEvent = 'connected' | 'disconnected' | 'entry_changed' | 'sync_complete' | 'error'

// ── Helpers ───────────────────────────────────────────────────────────────────

function _toBase64(bytes: Uint8Array): string {
  const g = globalThis as any
  if (g.Buffer) {
    return g.Buffer.from(bytes).toString('base64')
  }
  let binary = ''
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}

function _fromBase64(b64: string): Uint8Array {
  const g = globalThis as any
  if (g.Buffer) {
    return new Uint8Array(g.Buffer.from(b64, 'base64'))
  }
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

export class CircuitBreakerOpenError extends Error {
  constructor() {
    super('Circuit breaker is open')
    this.name = 'CircuitBreakerOpenError'
  }
}

// ── Client ────────────────────────────────────────────────────────────────────

export class GrimlockerClient {
  private readonly baseUrl: string
  private readonly token:   string
  private readonly _listeners: Map<string, Set<Function>>
  private _consecutiveFailures = 0
  private _circuitOpenUntil = 0
  private _isProbe = false
  private _ws?: WebSocket

  /**
   * @param baseUrl  Base URL of the daemon (e.g. "http://127.0.0.1:36353")
   * @param token    Authentication token (GRIMLOCKER_TOKEN from daemon stdout)
   */
  constructor(baseUrl: string, token: string) {
    this.baseUrl    = baseUrl.replace(/\/$/, '')
    this.token      = token
    this._listeners = new Map()
  }

  private _onSuccess() {
    this._consecutiveFailures = 0
    this._circuitOpenUntil = 0
    this._isProbe = false
  }

  private _onFailure() {
    if (this._isProbe) {
      this._circuitOpenUntil = Date.now() + 30_000
      this._isProbe = false
      return
    }
    this._consecutiveFailures++
    if (this._consecutiveFailures >= 5) {
      this._circuitOpenUntil = Date.now() + 30_000
      this._consecutiveFailures = 0
    }
  }

  // ── Event system ──────────────────────────────────────────────────────────

  /** Register an event listener. */
  on(event: GrimlockerEvent, listener: (...args: any[]) => void): void {
    if (event === 'entry_changed' || event === 'sync_complete' || event === 'error') {
      this._connectWs()
    }
    if (!this._listeners.has(event)) {
      this._listeners.set(event, new Set())
    }
    this._listeners.get(event)!.add(listener)
  }

  /** Remove an event listener. */
  off(event: string, listener: Function): void {
    this._listeners.get(event)?.delete(listener)
  }

  /** Async iterator over streaming events. */
  async *events(): AsyncIterableIterator<{ event: string; data: any }> {
    this._connectWs()
    const queue: { event: string; data: any }[] = []
    let notify: (() => void) | null = null

    const push = (ev: string, data: any) => {
      queue.push({ event: ev, data })
      notify?.()
    }

    const handlers = new Map<string, (...args: any[]) => void>()
    for (const ev of ['connected', 'disconnected', 'entry_changed', 'sync_complete', 'error']) {
      const fn = (...args: any[]) => push(ev, args[0])
      handlers.set(ev, fn)
      this.on(ev as GrimlockerEvent, fn)
    }

    try {
      while (true) {
        if (queue.length) {
          yield queue.shift()!
        } else {
          await new Promise<void>((r) => { notify = r })
          notify = null
        }
      }
    } finally {
      for (const [ev, fn] of handlers) {
        this.off(ev, fn)
      }
    }
  }

  private _connectWs() {
    if (this._ws) return
    const WS = (globalThis as any).WebSocket
    if (!WS) return
    const url = new URL(this.baseUrl)
    const wsUrl = `ws://${url.host}/ws?token=${encodeURIComponent(this.token)}`
    const ws: WebSocket = new WS(wsUrl)
    this._ws = ws

    ws.onopen = () => {
      this._emit('connected')
    }
    ws.onclose = () => {
      this._emit('disconnected')
      this._ws = undefined
    }
    ws.onerror = () => {
      this._emit('error', { message: 'WebSocket error' })
    }
    ws.onmessage = (msg) => {
      try {
        const data = JSON.parse(msg.data)
        const ev = data.event ?? 'message'
        if (ev === 'entry_changed' || ev === 'sync_complete' || ev === 'error') {
          this._emit(ev, data.payload ?? data)
        }
      } catch {
        this._emit('error', { message: 'Invalid WebSocket message' })
      }
    }
  }

  private _emit(event: string, ...args: any[]): void {
    this._listeners.get(event)?.forEach((fn) => {
      try { fn(...args) } catch { /* swallow listener errors */ }
    })
  }

  // ── Auth ──────────────────────────────────────────────────────────────────

  /** Unlock the vault with the master password. */
  async unlockVault(password: string): Promise<void> {
    await this._call('vault.unlock', { password })
    this._emit('connected')
  }

  /** Lock the vault (equivalent to auto-lock / logout). */
  async lockVault(): Promise<void> {
    await this._call('vault.logout', {})
    this._emit('disconnected')
  }

  /** Returns { initialized, unlocked } vault status. */
  async vaultStatus(): Promise<{ initialized: boolean; unlocked: boolean }> {
    return this._call('vault.status', {})
  }

  // ── Entries ───────────────────────────────────────────────────────────────

  /** List all vault entries (metadata only, no sensitive fields). */
  async listEntries(category?: EntryCategory): Promise<VaultEntry[]> {
    const payload = category ? { category } : {}
    const result: any = await this._call('entry.list', payload)
    return (result.entries ?? result) as VaultEntry[]
  }

  /** Read a single entry by ID (metadata only). */
  async getEntry(id: string): Promise<VaultEntry> {
    return this._call('entry.read', { id })
  }

  /** Create a generic vault entry. */
  async createEntry(
    title:    string,
    category: EntryCategory,
    fields:   Record<string, string>,
  ): Promise<VaultEntry> {
    return this._call('entry.create', { title, category, fields })
  }

  /** Create a new password entry. */
  async createPassword(
    title:    string,
    username: string,
    password: string,
    url?:     string,
    notes?:   string,
  ): Promise<VaultEntry> {
    return this._call('entry.create', {
      category: 'PASSWORD',
      title,
      fields: { username, password, url: url ?? '', notes: notes ?? '' },
    })
  }

  /** Create a new SSH key entry. */
  async createSSHKey(
    title:      string,
    publicKey:  string,
    privateKey: string,
    username?:  string,
  ): Promise<VaultEntry> {
    return this._call('entry.create', {
      category: 'SSH_KEY',
      title,
      fields: { public_key: publicKey, private_key: privateKey, username: username ?? '' },
    })
  }

  /** Update an existing entry's fields. */
  async updateEntry(id: string, fields: Record<string, string>): Promise<void> {
    await this._call('entry.update', { id, fields })
  }

  /** Delete an entry by ID. */
  async deleteEntry(id: string): Promise<void> {
    await this._call('entry.delete', { id })
  }

  /** Create multiple entries in parallel. */
  async createEntriesBatch(
    entries: Array<{ title: string; category: EntryCategory; fields: Record<string, string> }>,
  ): Promise<string[]> {
    const ids = await Promise.all(
      entries.map((e) => this.createEntry(e.title, e.category, e.fields).then((entry) => entry.id)),
    )
    return ids
  }

  /** Delete multiple entries in parallel. */
  async deleteEntriesBatch(ids: string[]): Promise<void> {
    await Promise.all(ids.map((id) => this.deleteEntry(id)))
  }

  // ── Certificates ──────────────────────────────────────────────────────────

  /** Create a new certificate entry with domain, cert, and private key. */
  async createCertificate(
    title:      string,
    domain:     string,
    cert:       string,
    privateKey: string,
  ): Promise<VaultEntry> {
    return this._call('entry.create', {
      category: 'CERTIFICATE',
      title,
      fields: { domain, certificate: cert, private_key: privateKey },
    })
  }

  /** List all password entries. */
  async listPasswords(): Promise<VaultEntry[]> {
    return this.listEntries('PASSWORD')
  }

  /** List all SSH key entries. */
  async listSshKeys(): Promise<VaultEntry[]> {
    return this.listEntries('SSH_KEY')
  }

  /** List all certificate entries. */
  async listCertificates(): Promise<CertificateEntry[]> {
    const result: any = await this._call('entry.list', { category: 'CERTIFICATE' })
    return (result.entries ?? result) as CertificateEntry[]
  }

  // ── Search ────────────────────────────────────────────────────────────────

  /** Full-text / field search across vault entries. */
  async searchEntries(query: string, category?: EntryCategory): Promise<VaultEntry[]> {
    const payload: Record<string, string> = { query }
    if (category) payload['category'] = category
    const result: any = await this._call('entry.search', payload)
    return (result.entries ?? result) as VaultEntry[]
  }

  // ── File Vault ────────────────────────────────────────────────────────────

  /** List the contents of a folder (pass empty string for root). */
  async listFolder(folderId = ''): Promise<FolderListing> {
    const payload = folderId ? { folder_id: folderId } : {}
    return this._call('file.list', payload)
  }

  /** Create a new folder, optionally under a parent. */
  async createFolder(name: string, parentId = ''): Promise<FolderItem> {
    const payload: Record<string, string> = { name }
    if (parentId) payload['parent_id'] = parentId
    return this._call('file.create_folder', payload)
  }

  /** Rename a folder by ID. */
  async renameFolder(id: string, name: string): Promise<void> {
    await this._call('file.rename_folder', { id, name })
  }

  /** Delete a folder by ID. */
  async deleteFolder(id: string): Promise<void> {
    await this._call('file.delete_folder', { id })
  }

  /** Move a file (identified by its manifest block ID) into a folder. */
  async moveFile(manifestBlockId: string, folderId: string): Promise<void> {
    await this._call('file.move', { manifest_block_id: manifestBlockId, folder_id: folderId })
  }

  /**
   * Upload binary or text content to the file vault.
   *
   * @param data       Raw bytes or a UTF-8 string.
   * @param fileName   Human-readable file name.
   * @param mimeType   MIME type (defaults to application/octet-stream).
   * @param folderId   Target folder ID (empty for root).
   * @param onProgress  Called at start and end with byte progress.
   */
  async uploadFile(
    data:       Uint8Array | string,
    fileName:   string,
    mimeType    = 'application/octet-stream',
    folderId    = '',
    onProgress?: (p: UploadProgress) => void,
  ): Promise<FileEntry> {
    const bytes = typeof data === 'string' ? new TextEncoder().encode(data) : data
    onProgress?.({ bytes_sent: 0, total_bytes: bytes.length })
    const b64 = _toBase64(bytes)
    const payload: Record<string, string> = {
      data_b64:  b64,
      file_name: fileName,
      mime_type: mimeType,
    }
    if (folderId) payload['folder_id'] = folderId
    const result = await this._call('file.ingest', payload)
    onProgress?.({ bytes_sent: bytes.length, total_bytes: bytes.length })
    return result as FileEntry
  }

  /**
   * Download a file from the vault by its manifest block ID.
   * Returns the raw file bytes.
   */
  async downloadFile(manifestBlockId: string): Promise<Uint8Array> {
    const result: any = await this._call('file.download', { manifest_block_id: manifestBlockId })
    return _fromBase64(result.data_b64)
  }

  // ── Sync ──────────────────────────────────────────────────────────────────

  /** List discovered LAN sync peers and sync metadata. */
  async listSyncPeers(): Promise<SyncStatus> {
    return this._call('sync.list_peers', {})
  }

  /** Trigger an immediate LAN sync cycle. Non-blocking on the daemon side. */
  async triggerSync(): Promise<void> {
    await this._call('sync.trigger', {})
  }

  // ── Audit ─────────────────────────────────────────────────────────────────

  /** Fetch the last n entries from the security audit log (default: 50). */
  async listAuditLog(n = 50): Promise<AuditEvent[]> {
    const result: any = await this._call('audit.list', { n })
    return (result.events ?? result) as AuditEvent[]
  }

  // ── Workspaces ────────────────────────────────────────────────────────────

  /** List all workspaces. */
  async listWorkspaces(): Promise<Workspace[]> {
    return this._call('workspace.list', {})
  }

  /** Create a new workspace. */
  async createWorkspace(name: string): Promise<{ id: string; name: string }> {
    return this._call('workspace.create', { name })
  }

  /** Switch to a workspace by ID. */
  async switchWorkspace(id: string): Promise<void> {
    await this._call('workspace.switch', { id })
  }

  /** Rename a workspace by ID. */
  async renameWorkspace(id: string, name: string): Promise<void> {
    await this._call('workspace.rename', { id, name })
  }

  /** Delete a workspace by ID. */
  async deleteWorkspace(id: string): Promise<void> {
    await this._call('workspace.delete', { id })
  }

  // ── Health + Recovery + Tools ─────────────────────────────────────────────

  /** Retrieve daemon health status and vault state. */
  async healthCheck(): Promise<HealthStatus> {
    return this._call('vault.status', {})
  }

  /** Return the recovery / mnemonic phrase for the vault. */
  async getRecoveryPhrase(password: string): Promise<string> {
    const result: any = await this._call('vault.recovery_phrase', { password })
    return result.recovery_phrase ?? result.phrase ?? ''
  }

  /**
   * Request the daemon to generate an SSH key pair.
   * If saveToVault is true the key pair is stored as a vault entry.
   */
  async generateSSHKey(
    comment      = '',
    saveToVault  = true,
  ): Promise<{ public_key: string; fingerprint: string; entry_id?: string }> {
    return this._call('tool.ssh_keygen', { comment, save_to_vault: saveToVault })
  }

  // ── Internal ──────────────────────────────────────────────────────────────

  private async _call<T = unknown>(action: string, payload: unknown): Promise<T> {
    if (this._circuitOpenUntil > 0) {
      if (Date.now() < this._circuitOpenUntil) {
        throw new CircuitBreakerOpenError()
      }
      this._isProbe = true
    }

    let attempt = 0
    let delayMs = 100
    let lastError: any

    while (true) {
      try {
        const res = await fetch(`${this.baseUrl}/api/v1`, {
          method:  'POST',
          headers: {
            'Content-Type':      'application/json',
            'X-Grimlocker-Token': this.token,
          },
          body: JSON.stringify({ action, payload }),
        })

        const body = await res.json().catch(() => ({ error: res.statusText }))

        if (res.status >= 400 && res.status < 500) {
          const err: GrimlockerError = {
            message:     body?.error ?? body?.message ?? `HTTP ${res.status}`,
            action,
            status_code: res.status,
          }
          this._emit('error', err)
          throw err
        }

        if (!res.ok) {
          lastError = {
            message:     body?.error ?? body?.message ?? `HTTP ${res.status}`,
            action,
            status_code: res.status,
          }
        } else {
          this._onSuccess()
          return body as T
        }
      } catch (e: any) {
        if (e?.status_code >= 400 && e?.status_code < 500) {
          this._onFailure()
          throw e
        }
        lastError = e
      }

      if (attempt >= 3) break
      await new Promise(r => setTimeout(r, Math.min(delayMs, 2000)))
      delayMs *= 2
      attempt++
    }

    this._onFailure()
    throw lastError ?? new Error('Request failed after retries')
  }
}

export default GrimlockerClient
