/**
 * TauriBridge — Binary WebSocket-Client für den Grimlocker Go-Daemon.
 *
 * Das hier ist so ziemlich das Herzstück des Frontends. Es managed den gesamten
 * Lebenszyklus: Token finden → WS-Verbindung aufbauen → 3-Wege-Handshake
 * (INIT.READY → AUTH.TOKEN_SUBMIT → KERNEL.STATE_READY) → Request/Response
 * übers binäre Protokoll. Plus Auto-Reconnect mit exponentiellem Backoff,
 * falls die Verbindung reisst.
 *
 * Wire-Format: [4-Byte Big-Endian Länge][1-Byte msgType][Payload]
 * Alle Methoden geben Promises zurück, die entweder auf das ACK des Daemons
 * (oder den passenden Response-Typ) resolven oder mit MSG_ERROR rejecten.
 */
import { useGrimStore } from '../store/useGrimStore'
import { decryptSKE, base64ToBytes } from './crypto'

// ─── Nachrichtentyp-Konstanten (binäres Protokoll) ────────────────────────────
// Jeder Message-Type ist ein einzelnes Byte. Die namen spiegeln die Go-Seite.
// Wichtig fürs Debugging: die hex-Werte müssen mit dem Daemon übereinstimmen.

const MSG_GET_HEADER            = 0x01
const MSG_HEADER                = 0x02
const MSG_GET_CIPHERTEXT        = 0x03
const MSG_CIPHERTEXT            = 0x04
const MSG_UPDATE_HEADER         = 0x05
const MSG_UPDATE_CIPHERTEXT     = 0x06
const MSG_TRIGGER_WIPE          = 0x07
const MSG_ACK                   = 0x08
const MSG_ERROR                 = 0x09
const MSG_PANIC_WIPE            = 0x0A
const MSG_GENERATE_MATRIX       = 0x0B
const MSG_PROGRESS              = 0x0C
const MSG_GENERATION_RESULT     = 0x0D
const MSG_ZEROIZE_CONFIRM       = 0x0E
const MSG_INITIALIZE_VAULT      = 0x0F
const MSG_UNLOCK_VAULT          = 0x10
const MSG_SAVE_ENTRY            = 0x11
const MSG_RECOVERY_PHRASE       = 0x12
const MSG_UNLOCK_RESULT         = 0x13
const MSG_CHECK_VAULT_STATUS    = 0x14
const MSG_LIST_ENTRIES          = 0x15
const MSG_GET_ENTRY             = 0x16
const MSG_DELETE_ENTRY          = 0x17
const MSG_ENTRIES_RESULT        = 0x18
const MSG_ENTRY_DATA            = 0x19
const MSG_RESET_VAULT           = 0x1A
const MSG_LOG_BROADCAST         = 0x1B
const MSG_ENTRY_CREATE          = 0x1C
const MSG_ENTRY_RESULT          = 0x1D
const MSG_ENTRY_UPDATE          = 0x1E
const MSG_ENTRY_DELETE          = 0x1F
const MSG_FILE_INGEST_BEGIN     = 0x20
const MSG_FILE_CHUNK            = 0x21
const MSG_FILE_INGEST_END       = 0x22
const MSG_INGEST_PROGRESS       = 0x23
const MSG_GET_RECOVERY_PHRASE   = 0x24
const MSG_RECOVERY_PHRASE_DATA  = 0x25
const MSG_PANIC_WIPE_REQUEST    = 0x26
const MSG_WORKSPACE_LIST        = 0x27
const MSG_WORKSPACE_CREATE      = 0x28
const MSG_WORKSPACE_SWITCH      = 0x29
const MSG_WORKSPACE_DELETE      = 0x2A
const MSG_WORKSPACES_RESULT     = 0x2B

// Session-Key Encrypted Data (SKE) – Der Daemon verschlüsselt sensitive
// Entry-Daten mit einem Pro-Session-ChaCha20-Poly1305-Key, bevor er sie
// übers WebSocket schickt. Das Frontend hat den gleichen Key im RAM
// (nie persisted!) und entschlüsselt lokal.
const MSG_DECRYPT_ENTRY   = 0x33
const MSG_DECRYPTED_DATA  = 0x34

// Kategorie-gefilterte Entry-Queries (Omega+ Feature)
const MSG_ENTRY_QUERY        = 0x35
const MSG_ENTRY_QUERY_RESULT = 0x36

// SSH-Key-Generierung — TOOL-Channel (Omega+)
const MSG_SSH_KEY_GEN    = 0x37
const MSG_SSH_KEY_RESULT = 0x38

// Handshake-Protokoll — die ersten Messages nach dem WebSocket-Connect
const MSG_INIT_READY            = 0x2C
const MSG_AUTH_TOKEN_SUBMIT     = 0x2D
const MSG_KERNEL_STATE_READY    = 0x2E
const MSG_SYSTEM_HEARTBEAT      = 0x2F
const MSG_SYSTEM_ERROR          = 0x30
const MSG_SYSTEM_LOG            = 0x31

// Auth-Lifecycle — Logout, um den Session-Key zu killen
const MSG_AUTH_LOGOUT           = 0x3F
const MSG_AUTH_LOGOUT_ACK       = 0x40

// FileVault-Download — Chunked Download, analog zum Upload-Protokoll
const MSG_FILE_DOWNLOAD_REQUEST = 0x41
const MSG_FILE_CHUNK_DATA       = 0x42
const MSG_FILE_DOWNLOAD_END     = 0x43

// Workspace umbenennen
const MSG_WORKSPACE_RENAME      = 0x44

// Enterprise-Panic-Button — einmal gedrückt, ist alles weg
const MSG_PANIC_BUTTON          = 0x45
const MSG_SYSTEM_HEALTH_CHECK   = 0x32

// FileVault-Ordnersystem — hierarchische Ordner, analog zu Dateisystemen
const MSG_FOLDER_CREATE         = 0x60 // {name, parent_id}
const MSG_FOLDER_LIST           = 0x61 // {parent_id}
const MSG_FOLDER_RENAME         = 0x62 // {id, name}
const MSG_FOLDER_DELETE         = 0x63 // {id}
const MSG_FOLDER_RESULT         = 0x64 // server response
const MSG_FILE_MOVE_TO_FOLDER   = 0x65 // {manifest_block_id, folder_id}

// LAN Sync — Peer-Discovery und Sync über das lokale Netzwerk
const MSG_SYNC_LIST_PEERS       = 0x70 // {} — list discovered peers + sync metadata
const MSG_SYNC_TRIGGER          = 0x71 // {} — trigger immediate sync cycle
const MSG_SYNC_RESULT           = 0x72 // SKE-encrypted JSON sync state

// Audit-Log — Security-Events aus dem Daemon abrufen
const MSG_AUDIT_LIST            = 0x73 // [2-byte big-endian n] — request last n audit entries
const MSG_AUDIT_RESULT          = 0x74 // SKE-encrypted JSON []SecurityEvent

// Air-Gap-Backup — Export, Peek (Phase 1), Authorize (Phase 2), Checksum
const MSG_BACKUP_EXPORT         = 0x80 // {dest_path, hardware_tether}
const MSG_BACKUP_PEEK           = 0x81 // {source_path}
const MSG_BACKUP_AUTHORIZE      = 0x82 // {session_id, merge}
const MSG_BACKUP_CHECKSUM       = 0x83 // {path}
const MSG_BACKUP_RESULT         = 0x84 // JSON result (ExportResult|PeekResult|AuthorizeResult|ChecksumResult)

/** Menschenlesbare Namen für jeden Message-Type — wird fürs Throughput-Logging gebraucht. */
const OP_NAMES = {
  0x01: 'GET_HEADER',
  0x02: 'HEADER',
  0x03: 'GET_CIPHERTEXT',
  0x04: 'CIPHERTEXT',
  0x05: 'UPDATE_HEADER',
  0x06: 'UPDATE_CIPHERTEXT',
  0x07: 'TRIGGER_WIPE',
  0x08: 'ACK',
  0x09: 'ERROR',
  0x0A: 'PANIC_WIPE',
  0x0B: 'GENERATE_MATRIX',
  0x0C: 'PROGRESS',
  0x0D: 'GENERATION_RESULT',
  0x0E: 'ZEROIZE_CONFIRM',
  0x0F: 'INITIALIZE_VAULT',
  0x10: 'UNLOCK_VAULT',
  0x11: 'SAVE_ENTRY',
  0x12: 'RECOVERY_PHRASE',
  0x13: 'UNLOCK_RESULT',
  0x14: 'CHECK_VAULT_STATUS',
  0x15: 'LIST_ENTRIES',
  0x16: 'GET_ENTRY',
  0x17: 'DELETE_ENTRY',
  0x18: 'ENTRIES_RESULT',
  0x19: 'ENTRY_DATA',
  0x1A: 'RESET_VAULT',
  0x1B: 'LOG_BROADCAST',
  0x1C: 'ENTRY_CREATE',
  0x1D: 'ENTRY_RESULT',
  0x1E: 'ENTRY_UPDATE',
  0x1F: 'ENTRY_DELETE',
  0x20: 'FILE_INGEST_BEGIN',
  0x21: 'FILE_CHUNK',
  0x22: 'FILE_INGEST_END',
  0x23: 'INGEST_PROGRESS',
  0x24: 'GET_RECOVERY_PHRASE',
  0x25: 'RECOVERY_PHRASE_DATA',
  0x26: 'PANIC_WIPE_REQUEST',
  0x27: 'WORKSPACE_LIST',
  0x28: 'WORKSPACE_CREATE',
  0x29: 'WORKSPACE_SWITCH',
  0x2A: 'WORKSPACE_DELETE',
  0x2B: 'WORKSPACES_RESULT',
  0x2C: 'INIT_READY',
  0x2D: 'AUTH_TOKEN_SUBMIT',
  0x2E: 'KERNEL_STATE_READY',
  0x2F: 'SYSTEM_HEARTBEAT',
  0x30: 'SYSTEM_ERROR',
  0x31: 'SYSTEM_LOG',
  0x32: 'SYSTEM_HEALTH_CHECK',
  0x33: 'DECRYPT_ENTRY',
  0x34: 'DECRYPTED_DATA',
  0x35: 'ENTRY_QUERY',
  0x36: 'ENTRY_QUERY_RESULT',
  0x37: 'SSH_KEY_GEN',
  0x38: 'SSH_KEY_RESULT',
}

class TauriBridge {
  constructor() {
    this.socket = null
    this.connected = false
    this.token = null
    this.port = null
    /** @type {Map<number|string, Function[]>} Message-Type → Handler-Liste */
    this.handlers = new Map()
    /** @type {{ resolve: Function, reject: Function }|null} Hängt pending generateMatrix-Promise hier dran */
    this.pendingGenerate = null

    // Per-Session-ChaCha20-Poly1305-Key für SKE-Entschlüsselung.
    // Wird einmal beim Unlock gesetzt, lebt NUR im RAM, wird bei Disconnect/Lock genullt.
    this.sessionKey = null  // Uint8Array(32) oder null

    // Handshake-Status & Stabilität
    this.handshakeState = 'DISCONNECTED'
    this.isConnecting = false
    this.heartbeatTimer = null
    this.reconnectAttempts = 0
    this.maxReconnectAttempts = 15
    this.baseReconnectDelay = 2000
    this.maxReconnectDelay = 8000
    this.tokenPollInterval = null

    this._pendingCleanup = null

    window.addEventListener('beforeunload', () => {
      this.disconnect()
    })

    // Visibility-Change-Hack: WebView2 pausiert JS-Timer, wenn das Fenster minimiert ist.
    // Wenn der User zurückkommt, darf der Heartbeat nicht sofort glauben, der Daemon sei weg.
    // Also resetten wir den Watchdog bei VisibilityChange → 'visible'.
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState === 'visible' && this.connected) {
        this._resetHeartbeatWatchdog()
      }
    })
  }

  // ─── Connection & Handshake ──────────────────────────────────────────────
  // Die wichtigste Phase: Token finden → Verbinden → Handshake → Ready.

  /**
   * Findet Session-Token und IPC-Port.
   * Versucht zuerst Tauri IPC (Desktop-App), dann URL-Query-Params (Dev/Browser).
   * @returns {Promise<string|null>} Der gefundene Token oder null, falls der Daemon noch nicht bereit ist.
   */
  async discoverToken() {
    if (typeof window !== 'undefined' && (window.__TAURI_INTERNALS__ || window.__TAURI__)) {
      const { invoke } = await import('@tauri-apps/api/core')
      try {
        const config = await invoke('get_session_token')
        if (config.token) this.token = config.token.trim()
        if (config.ipc_port) this.port = config.ipc_port
        console.log(`[TauriBridge] Token received via IPC, port ${this.port}`)
        return this.token
      } catch (err) {
        // "daemon_not_ready" — polling will retry
        return null
      }
    }

    const params = new URLSearchParams(window.location.search)
    const urlToken = params.get('token')
    if (urlToken) {
      this.token = urlToken.trim()
      const urlPort = params.get('port')
      if (urlPort) this.port = parseInt(urlPort, 10)
      return this.token
    }

    return null
  }

  /**
   * Öffnet eine WebSocket-Verbindung zum Go-Daemon.
   * Wenn ein Token mitgegeben wird, wird der direkt benutzt; sonst wird erst discoverToken()
   * aufgerufen. Falls kein Token existiert, wird über _waitForToken() alle 1s gepollt.
   * @param {string} [token] — Optionaler, bereits bekannter Session-Token.
   * @returns {Promise<void>}
   */
  async connect(token = null) {
    if (this.isConnecting || this.handshakeState === 'READY') {
      console.log('[TauriBridge] connect() skipped: already connecting or ready')
      return
    }

    if (token) {
      this.token = token
      return this._doConnect()
    }

    this.token = await this.discoverToken()

    if (!this.token) {
      console.log('[TauriBridge] Token not found, waiting for daemon to create it...')
      return this._waitForToken()
    }

    return this._doConnect()
  }

  /**
   * Pollt discoverToken() jede Sekunde, bis ein Token auftaucht oder wir aufgeben.
   * @returns {Promise<void>}
   */
  _waitForToken() {
    return new Promise((resolve, reject) => {
      let attempts = 0
      const maxAttempts = 20

      this.tokenPollInterval = setInterval(async () => {
        attempts++
        this.token = await this.discoverToken()

        if (this.token) {
          clearInterval(this.tokenPollInterval)
          this.tokenPollInterval = null
          console.log(`[TauriBridge] Token discovered on port ${this.port}, connecting...`)
          this._doConnect()
            .then(resolve)
            .catch(reject)
          return
        }

        if (attempts >= maxAttempts) {
          clearInterval(this.tokenPollInterval)
          this.tokenPollInterval = null
          reject(new Error('Daemon did not start. Token file never created.'))
        }
      }, 1000)
    })
  }

  /**
   * Baut die WebSocket-Verbindung auf und führt den 3-Wege-Handshake durch:
   *   1. WS connect (synchron)
   *   2. INIT.READY empfangen → AUTH.TOKEN_SUBMIT senden
   *   3. KERNEL.STATE_READY empfangen → Verbindung ist READY
   * Jede Phase hat ein 5s-Timeout — wenn der Daemon nicht antwortet, fliegt ein Fehler.
   * @returns {Promise<void>}
   */
  _doConnect() {
    return new Promise((resolve, reject) => {
      if (!this.port || !this.token) {
        this.isConnecting = false
        reject(new Error('Cannot connect: port or token not yet discovered'))
        return
      }

      if (this.socket && this.socket.readyState === WebSocket.CONNECTING) {
        console.warn('[TauriBridge] _doConnect skipped: socket already in CONNECTING state')
        return
      }

      this.isConnecting = true
      const wsUrl = `ws://127.0.0.1:${this.port}/ws?token=${encodeURIComponent(this.token)}`

      console.log(`[TauriBridge] Connecting to ${wsUrl}`)

      try {
        this.socket = new WebSocket(wsUrl)
      } catch (err) {
        this.isConnecting = false
        reject(new Error(`WebSocket creation failed: ${err.message}`))
        return
      }

      this.socket.binaryType = 'arraybuffer'

      let initReadyTimeout = null
      let stateReadyTimeout = null
      let initReadyHandler = null
      let stateReadyHandler = null
      let handshakeResolved = false

      const cleanup = () => {
        if (initReadyTimeout) { clearTimeout(initReadyTimeout); initReadyTimeout = null }
        if (stateReadyTimeout) { clearTimeout(stateReadyTimeout); stateReadyTimeout = null }
        if (initReadyHandler) { initReadyHandler(); initReadyHandler = null }
        if (stateReadyHandler) { stateReadyHandler(); stateReadyHandler = null }
      }

      this._pendingCleanup = cleanup

      this.socket.onopen = () => {
        this.reconnectAttempts = 0
        this._attachMessageHandler()
        this.handshakeState = 'SOCKET_CONNECTED'
        console.log('[TauriBridge] Socket open, waiting for INIT.READY...')

        this._resetHeartbeatWatchdog()

        // Phase 1: Wait for INIT.READY (5s timeout)
        initReadyTimeout = setTimeout(() => {
          if (!handshakeResolved) {
            cleanup()
            this.isConnecting = false
            reject(new Error('Kernel INIT.READY timeout'))
          }
        }, 5000)

        initReadyHandler = this.on(MSG_INIT_READY, () => {
          if (handshakeResolved) return
          clearTimeout(initReadyTimeout)
          initReadyHandler = null
          initReadyTimeout = null

          console.log('[TauriBridge] Handshake: received INIT.READY, sending TOKEN_SUBMIT')
          this.handshakeState = 'HANDSHAKING'

          try {
            const tokenPayload = JSON.stringify({ token: this.token })
            this._send(MSG_AUTH_TOKEN_SUBMIT, new TextEncoder().encode(tokenPayload))
          } catch (sendErr) {
            cleanup()
            this.isConnecting = false
            reject(new Error(`Failed to send AUTH.TOKEN_SUBMIT: ${sendErr.message}`))
            return
          }

          // Phase 2: Wait for KERNEL.STATE_READY (5s timeout)
          stateReadyTimeout = setTimeout(() => {
            if (!handshakeResolved) {
              cleanup()
              this.isConnecting = false
              reject(new Error('Kernel STATE_READY timeout'))
            }
          }, 5000)

          stateReadyHandler = this.on(MSG_KERNEL_STATE_READY, () => {
            if (handshakeResolved) return
            handshakeResolved = true
            clearTimeout(stateReadyTimeout)
            stateReadyHandler = null
            stateReadyTimeout = null
            this._pendingCleanup = null

            console.log('[TauriBridge] Handshake: received STATE_READY, connection READY')
            this.handshakeState = 'READY'
            this.connected = true
            this.isConnecting = false
            this.reconnectAttempts = 0
            useGrimStore.getState().setDaemonStatus('online')
            this._emit('connected')
            // Tier + Role aus /health laden (non-blocking, non-fatal)
            fetch(`http://127.0.0.1:${this.port}/health?token=${encodeURIComponent(this.token)}`)
              .then(r => r.json())
              .then(info => {
                useGrimStore.getState().setAppTier(info.tier || 'single')
                useGrimStore.getState().setUserRole(info.role || 'admin')
              })
              .catch(() => {})
            resolve()
          })
        })
      }

      this.socket.onclose = (event) => {
        cleanup()
        this._pendingCleanup = null
        this._clearHeartbeatWatchdog()
        this.handshakeState = 'DISCONNECTED'
        this.connected = false
        this.isConnecting = false
        useGrimStore.getState().setDaemonStatus('offline')
        this._emit('disconnected', { code: event.code, reason: event.reason })
        if (!handshakeResolved) {
          reject(new Error(`WebSocket closed before handshake complete (code: ${event.code})`))
        }
        this._attemptReconnect()
      }

      this.socket.onerror = (error) => {
        cleanup()
        this._pendingCleanup = null
        this._clearHeartbeatWatchdog()
        useGrimStore.getState().setDaemonStatus('error')
        this._emit('error', error)
        this.isConnecting = false
        if (!handshakeResolved) {
          reject(new Error('WebSocket connection failed'))
        }
      }
    })
  }

  /**
   * Versucht Reconnect mit exponentiellem Backoff (2s → 4s → 8s, max 15 Versuche).
   * Danach wird aufgegeben und 'reconnect_failed' emittiert.
   */
  _attemptReconnect() {
    if (this.isConnecting || this.handshakeState === 'READY' || this.reconnectAttempts >= this.maxReconnectAttempts) {
      if (this.reconnectAttempts >= this.maxReconnectAttempts) {
        this._emit('reconnect_failed')
      }
      return
    }

    this.reconnectAttempts++
    const delay = Math.min(
      this.baseReconnectDelay * Math.pow(2, this.reconnectAttempts - 1),
      this.maxReconnectDelay
    )

    this._emit('reconnecting', { attempt: this.reconnectAttempts, delay })

    setTimeout(async () => {
      if (!this.connected) {
        await this.discoverToken().catch(() => {})
        this._doConnect().catch(() => {})
      }
    }, delay)
  }

  /** Reset vom Heartbeat-Watchdog. Wird bei jeder eingehenden Message aufgerufen.
   * Timeout ist 30s, damit wir folgende Szenarien überleben:
   *   - File-Uploads (der Server skippt Heartbeats via TryLock während er beschäftigt ist)
   *   - Kurze WebView2-Throttling-Phasen (Fenster minimiert)
   */
  _resetHeartbeatWatchdog() {
    if (this.heartbeatTimer) clearTimeout(this.heartbeatTimer)
    this.heartbeatTimer = setTimeout(() => {
      console.warn('[TauriBridge] Heartbeat timeout (30s), triggering soft-reconnect')
      this._softReconnect()
    }, 30000)
  }

  /** Löscht den Heartbeat-Watchdog-Timer. Wird beim Disconnect aufgerufen. */
  _clearHeartbeatWatchdog() {
    if (this.heartbeatTimer) {
      clearTimeout(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }

  /** Schliesst den Socket und triggert _attemptReconnect() — sanfter Reconnect nach Heartbeat-Timeout. */
  _softReconnect() {
    if (this.socket) {
      this.socket.close()
      this.socket = null
    }
    this.connected = false
    this.handshakeState = 'DISCONNECTED'
    useGrimStore.getState().setDaemonStatus('offline')
    this._emit('disconnected', { reason: 'heartbeat_timeout' })
  }

  /** Hängt den onmessage-Handler an den WebSocket. Extra-Methode, weil wir auch den Session-Key-Fallback brauchen. */
  _attachMessageHandler() {
    if (this._sessionKeyFallback) {
      this._sessionKeyFallback()
      this._sessionKeyFallback = null
    }

    this.socket.onmessage = (event) => {
      this._processMessage(event.data)
    }

    this._sessionKeyFallback = this.on(MSG_UNLOCK_RESULT, (payload) => {
      try {
        const text = new TextDecoder().decode(payload)
        const parsed = JSON.parse(text)
        if (parsed.session_key) {
          const keyBytes = base64ToBytes(parsed.session_key)
          if (keyBytes.length === 32) {
            this.setSessionKey(keyBytes)
            console.log('[TauriBridge] Session key captured from UNSOLICITED UNLOCK_RESULT')
          }
        }
      } catch {
        // Not JSON or no session_key — the explicit unlockVault handler will deal with it
      }
    })
  }

  /**
   * Parst einen binären Message-Frame: [4-Byte Länge][1-Byte Type][Payload].
   * SYSTEM-Events werden sofort geroutet (und nicht ans normale Dispatch weitergereicht),
   * alle anderen gehen durch den normalen _dispatch.
   */
  _processMessage(buffer) {
    const data = new Uint8Array(buffer)

    if (data.length < 5) {
      this._emit('error', new Error('Message too short'))
      return
    }

    const view = new DataView(buffer)
    const msgLen = view.getUint32(0, false)

    if (data.length < 4 + msgLen) {
      this._emit('error', new Error('Incomplete message'))
      return
    }

    const msgType = data[4]
    const payload = data.slice(5, 4 + msgLen)

    // Any incoming message counts as a heartbeat reset
    this._resetHeartbeatWatchdog()

    // Route SYSTEM events
    if (msgType === MSG_SYSTEM_ERROR) {
      try {
        const text = new TextDecoder().decode(payload)
        const err = JSON.parse(text)
        console.error('[TauriBridge] SYSTEM.ERROR:', err.error || text)
        this._emit('system_error', err)
      } catch (_) {}
      return
    }
    if (msgType === MSG_SYSTEM_LOG) {
      try {
        const text = new TextDecoder().decode(payload)
        const logEntry = JSON.parse(text)
        console.log('[TauriBridge] SYSTEM.LOG:', logEntry)
        this._emit('system_log', logEntry)
      } catch (_) {}
      return
    }
    if (msgType === MSG_SYSTEM_HEALTH_CHECK) {
      console.log('[TauriBridge] SYSTEM.HEALTH_CHECK received')
      this._emit('health_check', {})
      return
    }

    const { addThroughputPoint, addOperation } = useGrimStore.getState()
    addThroughputPoint(buffer.byteLength)

    const opName = OP_NAMES[msgType] || `MSG_0x${msgType.toString(16).toUpperCase().padStart(2, '0')}`
    addOperation(opName, `${buffer.byteLength}B`)

    this._dispatch(msgType, payload)
  }

  /** Ruft alle registrierten Handler für einen Message-Type auf, plus etwaige 'any'-Wildcard-Handler. */
  _dispatch(msgType, payload) {
    const handlers = this.handlers.get(msgType) || []
    handlers.forEach(fn => {
      try {
        fn(payload)
      } catch (err) {
        console.error('[TauriBridge] Handler error:', err)
      }
    })

    const anyHandlers = this.handlers.get('any') || []
    anyHandlers.forEach(fn => {
      try {
        fn(msgType, payload)
      } catch (err) {
        console.error('[TauriBridge] Broadcast handler error:', err)
      }
    })
  }

  /**
   * Registriert einen Handler für einen bestimmten Message-Type (oder ein Named-Event wie 'connected').
   * @param {number|string} msgType — Numerischer Message-Type oder Event-Name-String.
   * @param {Function} handler — Callback, der das Payload kriegt.
   * @returns {Function} Unsubscribe-Funktion (einfach aufrufen zum Deregistrieren).
   */
  on(msgType, handler) {
    if (!this.handlers.has(msgType)) {
      this.handlers.set(msgType, [])
    }
    this.handlers.get(msgType).push(handler)

    return () => {
      const list = this.handlers.get(msgType)
      if (list) {
        const idx = list.indexOf(handler)
        if (idx !== -1) list.splice(idx, 1)
      }
    }
  }

  /** Emittiert ein Named-Event an alle registrierten Handler. */
  _emit(event, data) {
    const handlers = this.handlers.get(event) || []
    handlers.forEach(fn => fn(data))
  }

  /**
   * Baut den binären Wire-Frame: [4-Byte Big-Endian Länge][1-Byte Type][Payload].
   * @param {number} msgType — Message-Type-Byte.
   * @param {Uint8Array} [payload] — Optionale Payload-Bytes.
   * @returns {Uint8Array} Fertig gerahmte Message zum Senden.
   */
  _frameMessage(msgType, payload = new Uint8Array(0)) {
    const msgData = new Uint8Array(1 + payload.length)
    msgData[0] = msgType
    msgData.set(payload, 1)

    const len = msgData.length
    const frame = new Uint8Array(4 + len)
    const view = new DataView(frame.buffer)
    view.setUint32(0, len, false)
    frame.set(msgData, 4)

    return frame
  }

  /**
   * Sendet eine gerahmte binäre Message übers WebSocket.
   * @param {number} msgType — Message-Type-Byte.
   * @param {Uint8Array} [payload] — Optionale Payload-Bytes.
   * @throws {Error} Wenn der Socket nicht offen ist.
   */
  _send(msgType, payload = new Uint8Array(0)) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      throw new Error('TauriBridge socket not open')
    }
    const frame = this._frameMessage(msgType, payload)
    this.socket.send(frame)
  }

  /**
   * Speichert den Per-Session-ChaCha20-Poly1305-Key für SKE-Entschlüsselung.
   * Wird einmal nach erfolgreichem Unlock aufgerufen. Der Key lebt nur im JS-RAM
   * und wird bei Disconnect/Lock sicher genullt (siehe clearSessionKey).
   * @param {Uint8Array} key — 32-Byte Session-Key
   */
  setSessionKey(key) {
    this.sessionKey = key
  }

  /**
   * Löscht den Session-Key sicher. Wird bei Disconnect, Lock oder Page-Unload aufgerufen.
   * Nutzt Two-Pass-Zeroization (Random → Zero), damit V8 den fill(0) nicht wegoptimiert.
   */
  clearSessionKey() {
    if (this.sessionKey) {
      // Two-pass zeroization prevents V8 dead-store elimination of a single fill(0).
      // First overwrite with random data, then zero — both passes are observable
      // as side effects (the second depends on the first), so neither is elided.
      crypto.getRandomValues(this.sessionKey)
      this.sessionKey.fill(0)
      this.sessionKey = null
    }
  }

  // ─── Vault-Operationen ────────────────────────────────────────────────────
  // Alle direkten Interaktionen mit dem Vault: Status, Init, Unlock, CRUD.

  /**
   * Fragt den Vault-Status beim Daemon an.
   * @returns {Promise<{initialized: boolean, isV5: boolean}>} Vault-Status-Flags.
   */
  checkVaultStatus() {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const ackHandler = this.on(MSG_ACK, (payload) => {
        ackHandler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const status = JSON.parse(text)
          resolve({ initialized: status.initialized, isV5: status.isV5 })
        } catch (err) {
          resolve({ initialized: true, isV5: true })
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        ackHandler()
        errHandler()
        const msg = new TextDecoder().decode(payload)
        if (msg === 'not_initialized') {
          resolve({ initialized: false, isV5: false })
        } else {
          reject(new Error(msg))
        }
      })

      this._send(MSG_CHECK_VAULT_STATUS)
    })
  }

  /**
   * Initialisiert einen neuen Vault mit dem gegebenen Passwort.
   * Der Daemon erstellt Salt, Sentinel, Entropy-File und gibt eine 200-Zeichen-Recovery-Phrase zurück.
   * @param {string} password — Master-Passwort für den neuen Vault.
   * @returns {Promise<string>} Die generierte Recovery-Phrase (wird nur EINMAL angezeigt!).
   */
  initializeVault(password) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_RECOVERY_PHRASE, (payload) => {
        handler()
        const phrase = new TextDecoder().decode(payload)
        resolve(phrase)
      })

      const errorHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errorHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(password)
      this._send(MSG_INITIALIZE_VAULT, payload)
    })
  }

  /**
   * Entsperrt den Vault mit dem gegebenen Passwort.
   * Bei Erfolg wird der Session-Key aus der Response extrahiert und gespeichert —
   * der wird dann für die SKE-Entschlüsselung aller folgenden Entry-Daten gebraucht.
   * @param {string} password — Master-Passwort zum Entsperren.
   * @returns {Promise<{success: boolean, sessionKeyB64?: string}>}
   */
  unlockVault(password) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_UNLOCK_RESULT, (payload) => {
        handler()
        const result = new TextDecoder().decode(payload)

        // The unlock response JSON includes a session_key field (base64-encoded).
        // Try to extract and store it for SKE.
        try {
          const parsed = JSON.parse(result)
          if (parsed.session_key) {
            const keyBytes = base64ToBytes(parsed.session_key)
            if (keyBytes.length === 32) {
              this.setSessionKey(keyBytes)
            }
          }
          resolve({ success: result === 'success' || parsed.success === true })
        } catch {
          resolve({ success: result === 'success' })
        }
      })

      const errorHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errorHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(password)
      this._send(MSG_UNLOCK_VAULT, payload)
    })
  }

  /**
   * Speichert einen neuen Eintrag im verschlüsselten Vault.
   * Nimmt ein Entry-Object { type, title, username, password, ... } entgegen und
   * transformiert es in { type, data: { title, username, ... } } — das erwartet
   * der Go-Translator in wsSaveEntry. Der Daemon generiert die ID und verschlüsselt
   * den Payload mit dem MVK, bevor er in den Block-Store schreibt.
   * @param {Object} entry — Entry-Object mit mindestens einem `type`-Feld.
   * @returns {Promise<void>} Resolved beim ACK vom Daemon.
   */
  saveEntry(entry) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const { type, category, ...entryData } = entry
      delete entryData.id

      const payload = JSON.stringify({ type, category: category || type?.toUpperCase(), data: entryData })

      const handler = this.on(MSG_ACK, () => {
        handler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      this._send(MSG_SAVE_ENTRY, new TextEncoder().encode(payload))
    })
  }

  /**
    * Updated einen existierenden Vault-Eintrag.
    * Schickt das komplette Entry-Object (mit id, type, category, und geänderten Feldern)
    * via MSG_ENTRY_UPDATE. Der Daemon merged die Felder in den existierenden Eintrag.
    * @param {string} id — Die eindeutige ID des Eintrags.
    * @param {Object} updates — Felder zum Updaten { title, username, password, url, notes, ... }.
    * @returns {Promise<void>} Resolved beim ENTRY_RESULT-ACK vom Daemon.
    */
  updateEntry(id, updates) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const payload = JSON.stringify({ id, ...updates })

      const handler = this.on(MSG_ENTRY_RESULT, () => {
        handler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      this._send(MSG_ENTRY_UPDATE, new TextEncoder().encode(payload))
    })
  }

/**
   * Fordert die Liste aller Vault-Einträge vom Daemon an.
   * Der Daemon schickt SKE-verschlüsselte Metadaten — diese Methode entschlüsselt
   * lokal mit dem Session-Key und gibt die Klartext-Liste zurück.
   * @returns {Promise<Array<{id, type, title, created_at, updated_at}>>}
   */
  listEntries() {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ENTRIES_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const raw = JSON.parse(text)

          // SKE-encrypted response: { encrypted: "base64..." }
          if (raw.encrypted && this.sessionKey) {
            const plaintext = decryptSKE(raw.encrypted, this.sessionKey)
            const entries = JSON.parse(plaintext)
            resolve(entries)
          } else if (raw.encrypted && !this.sessionKey) {
            console.error('[TauriBridge] listEntries: SKE-encrypted data received but no session key available')
            reject(new Error('Session key not available — please unlock the vault'))
          } else {
            // Plaintext response (no SKE)
            resolve(Array.isArray(raw) ? raw : [raw])
          }
        } catch (err) {
          reject(new Error('Failed to parse entries: ' + err.message))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      this._send(MSG_LIST_ENTRIES)
    })
  }

  /**
   * Holt die Metadaten eines einzelnen Vault-Eintrags per ID.
   * Der Daemon schickt SKE-verschlüsselte Metadaten (keine sensitiven Daten).
   * Entschlüsselung lokal mit dem Session-Key.
   * @param {string} id — UUID des Eintrags.
   * @returns {Promise<Object>} Entschlüsselte Metadaten {id, type, title, created_at, updated_at}.
   */
  getEntry(id) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ENTRY_DATA, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const raw = JSON.parse(text)

          // SKE-encrypted response: { encrypted: "base64..." }
          if (raw.encrypted && this.sessionKey) {
            const plaintext = decryptSKE(raw.encrypted, this.sessionKey)
            const entry = JSON.parse(plaintext)
            resolve(entry)
          } else if (raw.encrypted && !this.sessionKey) {
            reject(new Error('Session key not available — please unlock the vault'))
          } else {
            // Plaintext fallback
            resolve(raw)
          }
        } catch (err) {
          reject(new Error('Failed to parse entry: ' + err.message))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(id)
      this._send(MSG_GET_ENTRY, payload)
    })
  }

  /**
   * Fordert die vollständige Entschlüsselung eines Vault-Eintrags an.
   * Achtung, das ist der EINZIGE Pfad, der Passwörter, SSH-Keys etc. im Klartext zeigt:
   * Der Daemon entschlüsselt mit dem MVK, verschlüsselt dann mit dem Session-Key (SKE)
   * neu und schickt es rüber. Das Frontend entschlüsselt SKE lokal.
   * @param {string} id — UUID des Eintrags.
   * @returns {Promise<Object>} Komplett entschlüsselter Eintrag {id, type, data: {...}, ...}.
   */
  decryptEntry(id) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }
      if (!this.sessionKey) {
        reject(new Error('Session key not available — unlock required'))
        return
      }

      const handler = this.on(MSG_DECRYPTED_DATA, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const raw = JSON.parse(text)

          // SKE-encrypted: { encrypted: "base64..." }
          if (raw.encrypted) {
            const plaintext = decryptSKE(raw.encrypted, this.sessionKey)
            const entry = JSON.parse(plaintext)
            resolve(entry)
          } else {
            reject(new Error('Unexpected plaintext in decrypt response'))
          }
        } catch (err) {
          reject(new Error('Failed to decrypt entry: ' + err.message))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(JSON.stringify({ id }))
      this._send(MSG_DECRYPT_ENTRY, payload)
    })
  }

  /**
   * Löscht einen Eintrag aus dem Vault.
   * @param {string} id — UUID des Eintrags.
   * @returns {Promise<void>} Resolved beim ACK vom Daemon.
   */
  deleteEntry(id) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ACK, () => {
        handler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(id)
      this._send(MSG_DELETE_ENTRY, payload)
    })
  }

  /**
   * Setzt (zerstört) den Vault mit der Recovery-Phrase zurück.
   * Löscht alle Vault-Dateien und setzt Metadaten auf nicht-initialisiert.
   * @param {string} phrase — Die 200-Zeichen-Recovery-Phrase.
   * @returns {Promise<void>}
   */
  resetVault(phrase) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ACK, () => {
        handler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(phrase)
      this._send(MSG_RESET_VAULT, payload)
    })
  }

  // ─── Matrix / Entropy-Generierung ─────────────────────────────────────────
  // Die Entropy-Matrix ist das Herz der Coordinate-basierten Authentifizierung.
  // 400.000 Zeilen kryptografischer Zufall, aus dem später Schlüssel abgeleitet werden.

  /**
   * Fordert die Generierung einer Entropy-Matrix-Datei vom Daemon an.
   * @param {number} [lineCount=400000] — Wie viele Zeilen generiert werden sollen.
   * @param {string} [entropyPath=''] — Optionaler eigener Pfad für die Entropy-Datei.
   * @returns {Promise<Object>} Generierungsergebnis mit key_hex, Koordinaten, etc.
   */
  generateMatrix(lineCount = 400000, entropyPath = '') {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      this.pendingGenerate = { resolve, reject }

      const request = { line_count: lineCount, entropy_path: entropyPath }
      const payload = new TextEncoder().encode(JSON.stringify(request))

      this._send(MSG_GENERATE_MATRIX, payload)
    })
  }

  /**
   * Registriert einen Handler für Generierungs-Fortschritts-Updates.
   * @param {Function} handler — Callback kriegt {progress, stage, message}.
   * @returns {Function} Unsubscribe-Funktion.
   */
  onProgress(handler) {
    return this.on(MSG_PROGRESS, (payload) => {
      try {
        const text = new TextDecoder().decode(payload)
        const progress = JSON.parse(text)
        handler(progress)
      } catch {
        handler({ progress: 0, stage: 'unknown', message: 'Invalid progress data' })
      }
    })
  }

  /**
   * Registriert einen Handler für Generierungsergebnisse.
   * Resolved auch das pending generateMatrix()-Promise.
   * @param {Function} handler — Callback kriegt das Result-Object.
   * @returns {Function} Unsubscribe-Funktion.
   */
  onGenerationResult(handler) {
    return this.on(MSG_GENERATION_RESULT, (payload) => {
      try {
        const text = new TextDecoder().decode(payload)
        const result = JSON.parse(text)

        if (this.pendingGenerate) {
          this.pendingGenerate.resolve(result)
          this.pendingGenerate = null
        }

        handler(result)
      } catch (err) {
        if (this.pendingGenerate) {
          this.pendingGenerate.reject(err)
          this.pendingGenerate = null
        }
      }
    })
  }

  /**
   * Registriert einen Handler für Fehlermeldungen vom Daemon.
   * Rejected auch das pending generateMatrix()-Promise, falls aktiv.
   * @param {Function} handler — Callback kriegt den Error-String.
   * @returns {Function} Unsubscribe-Funktion.
   */
  onError(handler) {
    return this.on(MSG_ERROR, (payload) => {
      const errorMsg = new TextDecoder().decode(payload)

      if (this.pendingGenerate) {
        this.pendingGenerate.reject(new Error(errorMsg))
        this.pendingGenerate = null
      }

      handler(errorMsg)
    })
  }

  // ─── Security-Operationen ─────────────────────────────────────────────────
  // Zeroize und Panic Wipe – die harten Geschütze. Zeroize bestätigt dem Daemon,
  // dass wir seinen Wipe-Befehl verarbeitet haben; Panic Wipe zerstört alles.

  /**
   * Sendet Zeroize-Bestätigung an den Daemon.
   * Bestätigt, dass das Frontend den Speicher-Wipe nach einem Panic Wipe abgeschlossen hat.
   */
  zeroizeConfirm() {
    this._send(MSG_ZEROIZE_CONFIRM)
  }

  /**
   * Löst einen Panic Wipe aus — zerstört alle Vault-Daten sicher.
   * Der Daemon überschreibt alle Dateien mit Rauschen und löscht die Schlüssel.
   * @returns {Promise<void>}
   */
  panicWipe() {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ACK, () => {
        handler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      this._send(MSG_PANIC_WIPE_REQUEST)
    })
  }

/**
    * Holt die verschlüsselte Recovery-Phrase mit dem Master-Passwort.
    * Falls der Daemon SKE-verschlüsselte Daten schickt (was er in neueren Versionen tut),
    * entschlüsseln wir lokal.
    * @param {string} password — Master-Passwort zum Entschlüsseln der gespeicherten Phrase.
    * @returns {Promise<string>} Die 200-Zeichen-Recovery-Phrase im Klartext.
    */
  getRecoveryPhrase(password) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_RECOVERY_PHRASE_DATA, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          // Try SKE-encrypted format: { encrypted: "base64..." }
          const parsed = JSON.parse(text)
          if (parsed.encrypted && this.sessionKey) {
            const plaintext = decryptSKE(parsed.encrypted, this.sessionKey)
            resolve(plaintext)
            return
          }
          // Plaintext fallback
          resolve(text)
        } catch {
          // Not JSON — treat as plaintext phrase
          resolve(new TextDecoder().decode(payload))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(password)
      this._send(MSG_GET_RECOVERY_PHRASE, payload)
    })
  }

  // ─── Log & Diagnose ──────────────────────────────────────────────────────

  /**
   * Registriert einen Handler für Broadcast-Log-Messages vom Daemon.
   * @param {Function} handler — Callback kriegt den Log-Text.
   * @returns {Function} Unsubscribe-Funktion.
   */
  onLog(handler) {
    return this.on(MSG_LOG_BROADCAST, (payload) => {
      try {
        const text = new TextDecoder().decode(payload)
        handler(text)
      } catch (err) {
        console.error('[TauriBridge] Log handler error:', err)
      }
    })
  }

  /**
   * Fordert den Security-Header vom Daemon an (fehlgeschlagene Versuche, Lockdown-Info).
   * One-Shot-Request; die Antwort kommt über den MSG_HEADER-Handler.
   */
  requestHeader() {
    this._send(MSG_GET_HEADER)
  }

  /**
   * Parst ein 26-Byte-binäres Header-Payload in ein strukturiertes Object.
   * @param {Uint8Array} payload — Rohdaten (26 Bytes).
   * @returns {{failedAttempts: number, lockdownTimestamp: number,
   *   overrideAttemptsLeft: number, monotonicBootTicks: number,
   *   wallclockLastSeen: number}}
   */
  parseHeader(payload) {
    if (payload.length !== 26) {
      throw new Error(`Invalid header size: ${payload.length}`)
    }
    const view = new DataView(payload.buffer, payload.byteOffset, payload.byteLength)
    return {
      failedAttempts: payload[0],
      lockdownTimestamp: Number(view.getBigUint64(1, false)),
      overrideAttemptsLeft: payload[9],
      monotonicBootTicks: Number(view.getBigUint64(10, false)),
      wallclockLastSeen: Number(view.getBigUint64(18, false)),
    }
  }

  /**
   * Signalisiert dem Daemon, den Vault sofort zu sperren.
   * Wird vom Auto-Lock-Timer und manuellem Logout benutzt.
   * @returns {Promise<void>}
   */
  sendAuthLogout() {
    return new Promise((resolve) => {
      if (!this.connected) {
        resolve()
        return
      }
      const handler = this.on(MSG_AUTH_LOGOUT_ACK, () => {
        handler()
        resolve()
      })
      this._send(MSG_AUTH_LOGOUT)
    })
  }

  /**
   * Aktiviert den PANIC BUTTON — sofortiger Hard-Lockdown.
   * Braucht Admin-Passphrase zur Bestätigung. Nur für Admins.
   * @param {string} passphrase — Admin-Passphrase zur Verifikation.
   * @returns {Promise<void>}
   */
  activatePanicButton(passphrase) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      // Listen for either ACK (lockdown initiated) or ERROR.
      const ackHandler = this.on(MSG_ACK, () => {
        ackHandler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        ackHandler()
        errHandler()
        let msg = 'Panic activation failed'
        try { msg = JSON.parse(new TextDecoder().decode(payload)).message || msg } catch { /* ignore */ }
        reject(new Error(msg))
      })

      const payloadBytes = new TextEncoder().encode(JSON.stringify({ passphrase }))
      this._send(MSG_PANIC_BUTTON, payloadBytes)
    })
  }

  // ─── Workspace-Management ─────────────────────────────────────────────────
  // Workspaces sind isolierte Vault-Umgebungen — wie separate Profile.
  // Jeder Workspace hat eigene Einträge und Einstellungen.

  /**
   * Listet alle Workspaces vom Daemon auf.
   * @returns {Promise<Array<{id: string, name: string, active: boolean}>>}
   */
  listWorkspaces() {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_WORKSPACES_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const workspaces = JSON.parse(text)
          resolve(workspaces)
        } catch (err) {
          reject(new Error('Failed to parse workspaces'))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      this._send(MSG_WORKSPACE_LIST)
    })
  }

  /**
   * Erstellt einen neuen Workspace.
   * @param {string} name — Anzeigename für den neuen Workspace.
   * @returns {Promise<Object>} Das erstellte Workspace-Object.
   */
  createWorkspace(name) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_WORKSPACES_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const result = JSON.parse(text)
          resolve(result)
        } catch (err) {
          reject(new Error('Failed to parse workspace result'))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(JSON.stringify({ name }))
      this._send(MSG_WORKSPACE_CREATE, payload)
    })
  }

  /**
   * Wechselt zu einem anderen Workspace per ID.
   * Schickt nach dem Switch ein AUTH.LOGOUT, damit der User sich neu einloggen muss
   * (sonst hätte der neue Workspace noch den alten Session-Key).
   * @param {string} id — UUID des Ziel-Workspace.
   * @returns {Promise<Object>} Das Workspace-Object, zu dem gewechselt wurde.
   */
  switchWorkspace(id) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_WORKSPACES_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const ws = JSON.parse(new TextDecoder().decode(payload))
          resolve(ws)
        } catch (_) {
          resolve(null)
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(JSON.stringify({ id }))
      this._send(MSG_WORKSPACE_SWITCH, payload)
    })
  }

  /**
   * Löscht einen Workspace (inklusive aller darin enthaltenen Daten!).
   * @param {string} id — UUID des zu löschenden Workspace.
   * @returns {Promise<void>}
   */
  deleteWorkspace(id) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ACK, () => {
        handler()
        errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(JSON.stringify({ id }))
      this._send(MSG_WORKSPACE_DELETE, payload)
    })
  }

  /**
   * Benennt einen Workspace um.
   * @param {string} id      — UUID des umzubenennenden Workspace.
   * @param {string} name    — Neuer Anzeigename.
   * @returns {Promise<Array>} Aktualisierte Workspace-Liste.
   */
  renameWorkspace(id, name) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_WORKSPACES_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          resolve(JSON.parse(text))
        } catch {
          resolve([])
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payloadBytes = new TextEncoder().encode(JSON.stringify({ id, name }))
      this._send(MSG_WORKSPACE_RENAME, payloadBytes)
    })
  }

  /**
   * Lädt eine Datei aus dem FileVault per Manifest-Block-ID herunter.
   * Streamt ent-schlüsselte Chunks und setzt sie zu einem ArrayBuffer zusammen.
   * @param {string} manifestBlockId — z.B. "blob-{uuid}-manifest"
   * @returns {Promise<{data: ArrayBuffer, fileName: string, mimeType: string, sha256: string}>}
   */
  downloadFile(manifestBlockId) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const chunks = []

      const chunkHandler = this.on(MSG_FILE_CHUNK_DATA, (payload) => {
        // Each chunk is raw binary — accumulate in order.
        chunks.push(new Uint8Array(payload))
      })

      const endHandler = this.on(MSG_FILE_DOWNLOAD_END, (payload) => {
        chunkHandler()
        endHandler()
        errHandler()
        try {
          const meta = JSON.parse(new TextDecoder().decode(payload))
          // Assemble all chunks into a single ArrayBuffer.
          const totalLen = chunks.reduce((sum, c) => sum + c.length, 0)
          const combined = new Uint8Array(totalLen)
          let offset = 0
          for (const chunk of chunks) {
            combined.set(chunk, offset)
            offset += chunk.length
          }
          resolve({
            data: combined.buffer,
            fileName: meta.file_name || 'download',
            mimeType: meta.mime_type || 'application/octet-stream',
            sha256: meta.sha256 || '',
          })
        } catch (e) {
          reject(new Error('Failed to parse download metadata: ' + e.message))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        chunkHandler()
        endHandler()
        errHandler()
        let msg = 'Download failed'
        try {
          const parsed = JSON.parse(new TextDecoder().decode(payload))
          msg = parsed.message || parsed.error || msg
        } catch {
          msg = new TextDecoder().decode(payload)
        }
        reject(new Error(msg))
      })

      const payloadBytes = new TextEncoder().encode(
        JSON.stringify({ manifest_block_id: manifestBlockId })
      )
      this._send(MSG_FILE_DOWNLOAD_REQUEST, payloadBytes)
    })
  }

  /**
   * Öffnet eine Datei extern mit der Standard-App des Systems.
   * Schreibt die Daten in eine Temp-Datei, öffnet sie, und löscht die Temp-Datei
   * nach 30 Sekunden sicher (der User hatte ja Zeit, sie in der App zu öffnen).
   * @param {ArrayBuffer} data      — Dateiinhalt
   * @param {string}      filename  — Vorgeschlagener Dateiname
   * @param {string}      mimeType  — MIME-Type (nur zur Info, OS-Level egal)
   * @returns {Promise<void>}
   */
  async openFileExternally(data, filename, mimeType) {
    const { invoke } = await import('@tauri-apps/api/core')
    const bytes = Array.from(new Uint8Array(data))
    const tmpPath = await invoke('save_temp_file', { filename, data: bytes })
    await invoke('open_with_default_app', { path: tmpPath })

    // Nach 30s sichere Löschung der Temp-Datei — der User hatte Zeit zum Öffnen.
    setTimeout(async () => {
      try {
        await invoke('secure_delete_temp', { path: tmpPath })
      } catch (e) {
        console.warn('[tauriBridge] secure_delete_temp failed:', e)
      }
    }, 30_000)

    return tmpPath
  }

  // ─── Omega+ Operationen ──────────────────────────────────────────────────
  // Diese Features sind in der Omega+-Edition verfügbar: Entry-Queries,
  // SSH-Key-Generierung, FileVault und mehr.

  /**
   * Fragt Vault-Einträge nach Kategorie gefiltert ab.
   * @param {string} category — "PASSWORD"|"SSH_KEY"|"CERTIFICATE"|"FILE_VAULT"|"" (alle)
   * @returns {Promise<{category: string, entries: Array, count: number}>}
   */
  queryEntries(category = '') {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_ENTRY_QUERY_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const result = JSON.parse(text)
          if (result.error) {
            reject(new Error(result.error))
          } else {
            resolve(result)
          }
        } catch (err) {
          reject(new Error('Failed to parse entry query result: ' + err.message))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(JSON.stringify({ category }))
      this._send(MSG_ENTRY_QUERY, payload)
    })
  }

  /**
   * Generiert ein Ed25519-SSH-Keypair und speichert es optional im Vault.
   * @param {string} [comment] — Key-Kommentar (z.B. "user@host" oder ein Label).
   * @param {boolean} [saveToVault=true] — Ob das Keypair im Vault persistiert werden soll.
   * @param {string} [passphrase=''] — Optionale Passphrase für den Private-Key-PEM.
   * @param {boolean} [autoPassphrase=false] — Wenn true, generiert der Daemon eine sichere Zufallspassphrase.
   *   Die generierte Passphrase wird EINMAL in der Response zurückgegeben und nie im Vault gespeichert.
   * @returns {Promise<{public_key: string, fingerprint: string, entry_id: string, passphrase?: string}>}
   */
  generateSSHKey(comment = 'grimlocker-generated', saveToVault = true, passphrase = '', autoPassphrase = false) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const handler = this.on(MSG_SSH_KEY_RESULT, (payload) => {
        handler()
        errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const result = JSON.parse(text)
          if (result.error) {
            reject(new Error(result.error))
          } else {
            resolve(result)
          }
        } catch (err) {
          reject(new Error('Failed to parse SSH key result: ' + err.message))
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const payload = new TextEncoder().encode(JSON.stringify({
        comment,
        save_to_vault: saveToVault,
        passphrase,
        auto_passphrase: autoPassphrase,
      }))
      this._send(MSG_SSH_KEY_GEN, payload)
    })
  }

  /**
   * Lädt ein File-Object in den FileVault hoch (verschlüsselt).
   * Nutzt das 3-Message-Streaming-Protokoll: BEGIN → CHUNK(s) → END.
   * Die Chunks werden auf dem Weg cha-cha-verschlüsselt.
   * @param {File} file — Das File-Object zum Hochladen.
   * @param {Function} [onProgress] — Wird mit 0..1 Fortschritt aufgerufen.
   * @returns {Promise<Object>} Das BlobManifest, das der Daemon zurückgibt.
   */
  ingestFile(file, onProgress = null, folderId = '') {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const CHUNK_SIZE = 64 * 1024 // 64KB WebSocket chunks
      const INGEST_TIMEOUT = 120000 // 2 minutes

      // Timeout guard
      let cancelled = false
      const timeoutId = setTimeout(() => {
        cancelled = true
        cleanup()
        reject(new Error('File upload timed out'))
      }, INGEST_TIMEOUT)

      const cleanup = () => {
        clearTimeout(timeoutId)
        if (progressUnsub) progressUnsub()
      }

      // Step 1: Send BEGIN
      const beginPayload = JSON.stringify({
        file_name: file.name,
        mime_type: file.type || 'application/octet-stream',
        total_size: file.size,
        folder_id: folderId || '',
      })

      let progressUnsub = null

      const ackHandler = this.on(MSG_ACK, () => {
        ackHandler()
        errHandler()
        // Step 2: Stream chunks
        const reader = new FileReader()
        let offset = 0

        const sendNextChunk = () => {
          if (cancelled) return
          if (offset >= file.size) {
            // Step 3: Send END
            const endHandler = this.on(MSG_ENTRY_RESULT, (payload) => {
              endHandler()
              cleanup()
              try {
                const text = new TextDecoder().decode(payload)
                const manifest = JSON.parse(text)
                resolve(manifest)
              } catch (err) {
                reject(new Error('Failed to parse ingest manifest: ' + err.message))
              }
            })
            this._send(MSG_FILE_INGEST_END)
            return
          }

          const slice = file.slice(offset, Math.min(offset + CHUNK_SIZE, file.size))
          reader.readAsArrayBuffer(slice)
          reader.onload = (e) => {
            const chunkData = new Uint8Array(e.target.result)
            this._send(MSG_FILE_CHUNK, chunkData)
            offset += chunkData.length
            if (onProgress) onProgress(offset / file.size)
            sendNextChunk()
          }
          reader.onerror = () => {
            cleanup()
            reject(new Error('Failed to read file chunk'))
          }
        }

        sendNextChunk()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        ackHandler()
        errHandler()
        cleanup()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      if (onProgress) {
        progressUnsub = this.on(MSG_INGEST_PROGRESS, (payload) => {
          try {
            const text = new TextDecoder().decode(payload)
            const prog = JSON.parse(text)
            if (onProgress) onProgress(prog.progress || 0)
          } catch (_) {}
        })
      }

      this._send(MSG_FILE_INGEST_BEGIN, new TextEncoder().encode(beginPayload))
    })
  }

  // ─── Lifecycle ─────────────────────────────────────────────────────────────
  // Sauberes Aufräumen: Socket zu, Timer killen, Session-Key nullen.

  /**
   * Schliesst die WebSocket-Verbindung und räumt alle Timer und Handler auf.
   * Kann mehrfach aufgerufen werden — ist idempotent.
   */
  disconnect() {
    this._clearHeartbeatWatchdog()
    if (this._sessionKeyFallback) {
      this._sessionKeyFallback()
      this._sessionKeyFallback = null
    }
    if (this._pendingCleanup) {
      this._pendingCleanup()
      this._pendingCleanup = null
    }
    this.isConnecting = false
    this.handshakeState = 'DISCONNECTED'
    this.clearSessionKey()
    if (this.tokenPollInterval) {
      clearInterval(this.tokenPollInterval)
      this.tokenPollInterval = null
    }
    if (this.socket) {
      this.socket.close()
      this.socket = null
      this.connected = false
    }
  }

  // ── FileVault-Ordner-API ──────────────────────────────────────────────────
  // Hierarchische Ordnerstruktur für Dateien im Vault.

  /**
   * Erstellt einen Ordner in der FileVault-Hierarchie.
   * @param {string} name
   * @param {string} parentId  — "" für Root
   * @returns {Promise<object>} FolderEntry
   */
  createFolder(name, parentId = '') {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }
      const handler = this.on(MSG_FOLDER_RESULT, (payload) => {
        handler(); errHandler()
        const folder = this._decodeFolderResult(payload)
        if (folder.error) { reject(new Error(folder.error)); return }
        resolve(folder)
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_FOLDER_CREATE, new TextEncoder().encode(JSON.stringify({ name, parent_id: parentId })))
    })
  }

  /**
   * Listet den Inhalt (Ordner + Dateien) eines Ordners auf.
   * @param {string} parentId  — "" für Root
   * @returns {Promise<{parent_id, folders: [], files: []}>}
   */
  listFolder(parentId = '') {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }
      const handler = this.on(MSG_FOLDER_RESULT, (payload) => {
        handler(); errHandler()
        const result = this._decodeFolderResult(payload)
        resolve(result)
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_FOLDER_LIST, new TextEncoder().encode(JSON.stringify({ parent_id: parentId })))
    })
  }

  /**
   * Benennt einen Ordner um.
   * @param {string} id
   * @param {string} name
   */
  renameFolder(id, name) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }
      const handler = this.on(MSG_FOLDER_RESULT, (payload) => {
        handler(); errHandler()
        resolve(this._decodeFolderResult(payload))
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_FOLDER_RENAME, new TextEncoder().encode(JSON.stringify({ id, name })))
    })
  }

  /**
   * Löscht einen Ordner. Dateien im Ordner werden in den Parent verschoben (kein Datenverlust).
   * @param {string} id
   */
  deleteFolder(id) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }
      const handler = this.on(MSG_FOLDER_RESULT, (payload) => {
        handler(); errHandler()
        resolve(this._decodeFolderResult(payload))
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_FOLDER_DELETE, new TextEncoder().encode(JSON.stringify({ id })))
    })
  }

  /**
   * Verschiebt eine Datei in einen anderen Ordner.
   * @param {string} manifestBlockId
   * @param {string} folderId  — "" für Root
   */
  moveFileToFolder(manifestBlockId, folderId = '') {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }
      const handler = this.on(MSG_FOLDER_RESULT, (payload) => {
        handler(); errHandler()
        resolve(this._decodeFolderResult(payload))
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_FILE_MOVE_TO_FOLDER, new TextEncoder().encode(
        JSON.stringify({ manifest_block_id: manifestBlockId, folder_id: folderId })
      ))
    })
  }

  /**
   * Listet entdeckte LAN-Sync-Peers und Sync-Metadaten auf (last_sync_at, device_id).
   * Gibt SKE-entschlüsselt { peers: DiscoveredPeer[], last_sync_at: number, device_id: string } zurück.
   * Falls Sync nicht verfügbar ist, wird mit leerem peers-Array resolved (kein fataler Fehler).
   */
  listSyncPeers() {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_SYNC_RESULT, (payload) => {
        handler(); errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          // Plain-text trigger ack ({"ok":true}) — not the peers response, ignore
          const raw = JSON.parse(text)
          if (raw.ok) return  // trigger ack on same channel — wait for next
          if (raw.encrypted && this.sessionKey) {
            resolve(JSON.parse(decryptSKE(raw.encrypted, this.sessionKey)))
          } else {
            resolve(raw)
          }
        } catch (e) {
          const msg = e?.message ?? ''
          if (msg.includes('sync unavailable') || msg.includes('not valid JSON') || msg.includes('Unexpected token')) {
            resolve({ peers: [], last_sync_at: 0, device_id: '' })
          } else {
            reject(e)
          }
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        const msg = new TextDecoder().decode(payload)
        if (msg.includes('sync unavailable')) {
          resolve({ peers: [], last_sync_at: 0, device_id: '' })
        } else {
          reject(new Error(msg))
        }
      })

      this._send(MSG_SYNC_LIST_PEERS)
    })
  }

  /**
   * Löst sofort einen LAN-Sync-Zyklus aus.
   * Non-blocking auf Daemon-Seite; resolved, sobald der Daemon das Trigger-ACK schickt.
   */
  triggerSync() {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_SYNC_RESULT, (payload) => {
        handler(); errHandler()
        resolve()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        const msg = new TextDecoder().decode(payload)
        if (msg.includes('sync unavailable')) resolve()  // non-fatal
        else reject(new Error(msg))
      })

      this._send(MSG_SYNC_TRIGGER)
    })
  }

  // ── Air-Gap-Backup ────────────────────────────────────────────────────────

  /**
   * Exportiert den Vault in eine .grimbak-Datei.
   * @param {string} destPath — Ziel-Dateipfad
   * @param {boolean} hardwareTether — Backup an diese Hardware binden
   * @returns {Promise<{path:string, sha256:string, entry_count:number}>}
   */
  exportBackup(destPath, hardwareTether = false) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_BACKUP_RESULT, (payload) => {
        handler(); errHandler()
        try {
          const result = JSON.parse(new TextDecoder().decode(payload))
          if (result.error) { reject(new Error(result.error)); return }
          resolve(result)
        } catch (e) { reject(e) }
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_BACKUP_EXPORT, new TextEncoder().encode(
        JSON.stringify({ dest_path: destPath, hardware_tether: hardwareTether })
      ))
    })
  }

  /**
   * Phase 1 — Liest den Plaintext-Header einer .grimbak-Datei ohne Entschlüsselung.
   * @param {string} sourcePath — Quell-Dateipfad
   * @returns {Promise<PeekResult>}
   */
  peekBackup(sourcePath) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_BACKUP_RESULT, (payload) => {
        handler(); errHandler()
        try {
          const result = JSON.parse(new TextDecoder().decode(payload))
          if (result.error) { reject(new Error(result.error)); return }
          resolve(result)
        } catch (e) { reject(e) }
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_BACKUP_PEEK, new TextEncoder().encode(
        JSON.stringify({ source_path: sourcePath })
      ))
    })
  }

  /**
   * Phase 2 — Importiert ein Backup mit der session_id aus Phase 1 (peekBackup).
   * Vault muss entsperrt sein.
   * @param {string} sessionId — Session-ID aus peekBackup()
   * @param {boolean} merge — true = bestehende IDs überspringen; false = überschreiben
   * @returns {Promise<{imported_count:number, skipped_count:number}>}
   */
  authorizeBackup(sessionId, merge = false) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_BACKUP_RESULT, (payload) => {
        handler(); errHandler()
        try {
          const result = JSON.parse(new TextDecoder().decode(payload))
          if (result.error) { reject(new Error(result.error)); return }
          resolve(result)
        } catch (e) { reject(e) }
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_BACKUP_AUTHORIZE, new TextEncoder().encode(
        JSON.stringify({ session_id: sessionId, merge })
      ))
    })
  }

  /**
   * Berechnet SHA-256 über eine .grimbak-Datei (kein Vault-Unlock nötig).
   * @param {string} path — Dateipfad
   * @returns {Promise<{path:string, sha256:string}>}
   */
  checksumBackup(path) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_BACKUP_RESULT, (payload) => {
        handler(); errHandler()
        try {
          const result = JSON.parse(new TextDecoder().decode(payload))
          if (result.error) { reject(new Error(result.error)); return }
          resolve(result)
        } catch (e) { reject(e) }
      })
      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        reject(new Error(new TextDecoder().decode(payload)))
      })
      this._send(MSG_BACKUP_CHECKSUM, new TextEncoder().encode(
        JSON.stringify({ path })
      ))
    })
  }

  /**
   * Holt die letzten n Einträge aus dem Security-Audit-Log.
   * Gibt SKE-entschlüsselte SecurityEvent[] sortiert oldest-first zurück.
   * @param {number} n — Anzahl Einträge (default 50, max 500).
   */
  listAuditEntries(n = 50) {
    return new Promise((resolve, reject) => {
      if (!this.connected) { reject(new Error('Not connected')); return }

      const handler = this.on(MSG_AUDIT_RESULT, (payload) => {
        handler(); errHandler()
        try {
          const text = new TextDecoder().decode(payload)
          const raw = JSON.parse(text)
          if (raw.encrypted && this.sessionKey) {
            resolve(JSON.parse(decryptSKE(raw.encrypted, this.sessionKey)))
          } else {
            resolve(Array.isArray(raw) ? raw : [])
          }
        } catch (e) {
          reject(e)
        }
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        handler(); errHandler()
        const msg = new TextDecoder().decode(payload)
        if (msg.includes('audit unavailable')) resolve([])
        else reject(new Error(msg))
      })

      // Encode n as 2-byte big-endian
      const buf = new Uint8Array(2)
      new DataView(buf.buffer).setUint16(0, Math.min(Math.max(n, 1), 500), false)
      this._send(MSG_AUDIT_LIST, buf)
    })
  }

  /** Decodiert ein MsgFolderResult-Payload — verarbeitet sowohl SKE-verschlüsseltes als auch reines JSON. */
  _decodeFolderResult(payload) {
    try {
      const text = new TextDecoder().decode(payload)
      const parsed = JSON.parse(text)
      // SKE-encrypted response (same pattern as listEntries / fetchEntry)
      if (parsed.encrypted && this.sessionKey) {
        const plaintext = decryptSKE(parsed.encrypted, this.sessionKey)
        return JSON.parse(plaintext)
      }
      return parsed
    } catch (e) {
      console.error('[TauriBridge] _decodeFolderResult failed:', e)
      return {}
    }
  }
}

export const tauriBridge = new TauriBridge()
export {
  MSG_HEADER,
  MSG_CIPHERTEXT,
  MSG_ERROR,
  MSG_ACK,
  MSG_PROGRESS,
  MSG_GENERATION_RESULT,
  MSG_GENERATE_MATRIX,
  MSG_ZEROIZE_CONFIRM,
  MSG_GET_RECOVERY_PHRASE,
  MSG_RECOVERY_PHRASE_DATA,
  MSG_PANIC_WIPE_REQUEST,
  MSG_WORKSPACE_LIST,
  MSG_WORKSPACE_CREATE,
  MSG_WORKSPACE_SWITCH,
  MSG_WORKSPACE_DELETE,
  MSG_WORKSPACES_RESULT,
  MSG_SYSTEM_ERROR,
  MSG_SYSTEM_LOG,
  MSG_SYSTEM_HEALTH_CHECK,
  MSG_DECRYPT_ENTRY,
  MSG_DECRYPTED_DATA,
  MSG_ENTRY_QUERY,
  MSG_ENTRY_QUERY_RESULT,
  MSG_SSH_KEY_GEN,
  MSG_SSH_KEY_RESULT,
}