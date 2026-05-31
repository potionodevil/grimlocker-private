/**
 * TauriBridge — Binary WebSocket client for the Grimlocker Go daemon.
 *
 * Manages the full lifecycle: token discovery → WS connect → handshake
 * (INIT.READY → AUTH.TOKEN_SUBMIT → KERNEL.STATE_READY) → request/response
 * over the binary protocol, plus auto-reconnect with exponential backoff.
 *
 * Wire format: [4-byte big-endian length][1-byte msgType][payload]
 * All methods return Promises that resolve on the daemon's ACK (or matching
 * response type) and reject on MSG_ERROR.
 */
import { useGrimStore } from '../store/useGrimStore'
import { decryptSKE, base64ToBytes } from './crypto'

// ─── Message type constants (binary protocol) ───────────────────────────────

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

// Session-key encrypted data (SKE) — the daemon encrypts sensitive entry
// data with a per-session ChaCha20-Poly1305 key before sending it over WebSocket.
// The frontend holds the same key in RAM and decrypts locally.
const MSG_DECRYPT_ENTRY   = 0x33
const MSG_DECRYPTED_DATA  = 0x34

// Category-filtered entry queries (Omega+)
const MSG_ENTRY_QUERY        = 0x35
const MSG_ENTRY_QUERY_RESULT = 0x36

// SSH key generation — TOOL channel (Omega+)
const MSG_SSH_KEY_GEN    = 0x37
const MSG_SSH_KEY_RESULT = 0x38

// Handshake protocol
const MSG_INIT_READY            = 0x2C
const MSG_AUTH_TOKEN_SUBMIT     = 0x2D
const MSG_KERNEL_STATE_READY    = 0x2E
const MSG_SYSTEM_HEARTBEAT      = 0x2F
const MSG_SYSTEM_ERROR          = 0x30
const MSG_SYSTEM_LOG            = 0x31
const MSG_SYSTEM_HEALTH_CHECK   = 0x32

/** Human-readable names for each message type, used in throughput logging. */
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
    /** @type {Map<number|string, Function[]>} Message-type → handler list */
    this.handlers = new Map()
    /** @type {{ resolve: Function, reject: Function }|null} Pending generate-matrix promise */
    this.pendingGenerate = null

    // Per-session ChaCha20-Poly1305 key for SKE encryption.
    // Set once on unlock, held only in RAM, zeroed on disconnect/lock.
    this.sessionKey = null  // Uint8Array(32) or null

    // Handshake & stability
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
  }

  // ─── Connection & Handshake ──────────────────────────────────────────────

  /**
   * Discover the session token and IPC port.
   * Tries Tauri IPC first (desktop app), then URL query params (dev/browser).
   * @returns {Promise<string|null>} The discovered token, or null if not yet available.
   */
  async discoverToken() {
    if (typeof window !== 'undefined' && window.__TAURI__) {
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
   * Open a WebSocket connection to the Go daemon.
   * If a token is supplied it's used directly; otherwise discoverToken() is called first.
   * If no token is found, starts polling via _waitForToken().
   * @param {string} [token] — Optional pre-known session token.
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
   * Poll discoverToken() every second until a token is found or maxAttempts reached.
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
   * Establish the WebSocket and perform the 3-way handshake:
   *   1. WS connect
   *   2. Receive INIT.READY → send AUTH.TOKEN_SUBMIT
   *   3. Receive KERNEL.STATE_READY → connection is READY
   * Includes 5s timeouts for each handshake phase.
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
   * Attempt to reconnect with exponential backoff (2s → 4s → 8s).
   * Gives up after maxReconnectAttempts (15).
   */
  _attemptReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      this._emit('reconnect_failed')
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

  /** Reset the 5s heartbeat watchdog. Called on every incoming message. */
  _resetHeartbeatWatchdog() {
    if (this.heartbeatTimer) clearTimeout(this.heartbeatTimer)
    this.heartbeatTimer = setTimeout(() => {
      console.warn('[TauriBridge] Heartbeat timeout (5s), triggering soft-reconnect')
      this._softReconnect()
    }, 5000)
  }

  /** Clear the heartbeat watchdog timer. */
  _clearHeartbeatWatchdog() {
    if (this.heartbeatTimer) {
      clearTimeout(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }

  /** Close the socket and trigger _attemptReconnect(). Used on heartbeat timeout. */
  _softReconnect() {
    if (this.socket) {
      this.socket.close()
      this.socket = null
    }
    this.connected = false
    this.handshakeState = 'DISCONNECTED'
    useGrimStore.getState().setDaemonStatus('offline')
    this._emit('disconnected', { reason: 'heartbeat_timeout' })
    this._attemptReconnect()
  }

  /** Bind the WebSocket onmessage handler to _processMessage. */
  _attachMessageHandler() {
    this.socket.onmessage = (event) => {
      this._processMessage(event.data)
    }

    // Always capture session key from MSG_UNLOCK_RESULT, even if no explicit
    // listener is registered (e.g. after initializeVault's auto-unlock).
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
   * Parse a binary message frame: [4-byte length][1-byte type][payload].
   * Routes SYSTEM events immediately, then dispatches all others.
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

  /** Invoke all registered handlers for a message type, plus any 'any' wildcards. */
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
   * Register a handler for a specific message type (or a named event like 'connected').
   * @param {number|string} msgType — Numeric message type or event name string.
   * @param {Function} handler — Callback receiving the payload.
   * @returns {Function} Unsubscribe function.
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

  /** Emit a named event to all registered handlers. */
  _emit(event, data) {
    const handlers = this.handlers.get(event) || []
    handlers.forEach(fn => fn(data))
  }

  /**
   * Build the binary wire frame: [4-byte big-endian length][1-byte type][payload].
   * @param {number} msgType — Message type byte.
   * @param {Uint8Array} [payload] — Optional payload bytes.
   * @returns {Uint8Array} Framed message ready to send.
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
   * Send a framed binary message over the WebSocket.
   * @param {number} msgType — Message type byte.
   * @param {Uint8Array} [payload] — Optional payload bytes.
   * @throws {Error} If the socket is not open.
   */
  _send(msgType, payload = new Uint8Array(0)) {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      throw new Error('TauriBridge socket not open')
    }
    const frame = this._frameMessage(msgType, payload)
    this.socket.send(frame)
  }

  /**
   * Store the per-session ChaCha20-Poly1305 key used for SKE decryption.
   * Called once after a successful unlock. The key is held only in JS RAM
   * and is zeroed on disconnect/lock.
   * @param {Uint8Array} key — 32-byte session key
   */
  setSessionKey(key) {
    this.sessionKey = key
  }

  /**
   * Clear the session key. Called on disconnect, lock, or page unload.
   */
  clearSessionKey() {
    if (this.sessionKey) {
      this.sessionKey.fill(0)
      this.sessionKey = null
    }
  }

  // ─── Vault Operations ────────────────────────────────────────────────────

  /**
   * Send MSG_CHECK_VAULT_STATUS to the daemon.
   * @returns {Promise<{initialized: boolean, isV5: boolean}>} Vault status flags.
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
   * Initialize a new vault with the given password.
   * The daemon creates salt, sentinel, entropy file and returns a 200-char recovery phrase.
   * @param {string} password — Master password for the new vault.
   * @returns {Promise<string>} The generated recovery phrase (shown once).
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
   * Unlock the vault with the given password.
   * On success, extracts the session key from the response and stores it
   * for SKE decryption of subsequent entry data.
   * @param {string} password — Master password to unlock the vault.
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
   * Save an entry to the encrypted vault.
   * Accepts a single entry object { type, title, username, password, ... } and
   * transforms it into { type, data: { title, username, ... } } that the Go
   * translator's wsSaveEntry expects. The daemon generates the ID and encrypts
   * the payload with the MVK before writing to the block store.
   * @param {Object} entry — Entry object with at least a `type` field and
   *   arbitrary data fields (title, username, password, url, notes, etc.).
   * @returns {Promise<void>} Resolves on ACK from the daemon.
   */
  saveEntry(entry) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const { type, ...entryData } = entry
      delete entryData.id

      const payload = JSON.stringify({ type, data: entryData })

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
   * Request the list of all vault entries from the daemon.
   * The daemon returns SKE-encrypted metadata. This method decrypts
   * the data locally using the session key and returns the plaintext list.
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
   * Retrieve metadata for a single vault entry by its ID.
   * The daemon returns SKE-encrypted metadata (no sensitive data).
   * Decrypts locally with the session key.
   * @param {string} id — The entry's unique identifier (UUID).
   * @returns {Promise<Object>} Decrypted entry metadata {id, type, title, created_at, updated_at}.
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
   * Request full decryption of a vault entry by its ID.
   * The daemon decrypts the entry with the MVK, then re-encrypts with the
   * session key (SKE) before sending. The frontend decrypts SKE locally.
   * This is the ONLY path that reveals passwords, SSH keys, etc.
   * @param {string} id — The entry's unique identifier (UUID).
   * @returns {Promise<Object>} Decrypted entry with all fields {id, type, data: {...}, ...}.
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
   * Delete an entry from the vault by its ID.
   * @param {string} id — The entry's unique identifier (UUID).
   * @returns {Promise<void>} Resolves on ACK from the daemon.
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
   * Reset (destroy) the vault using the recovery phrase.
   * Wipes all vault files and resets metadata to uninitialized state.
   * @param {string} phrase — The 200-character recovery phrase.
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

  // ─── Matrix / Entropy Generation ─────────────────────────────────────────

  /**
   * Request generation of an entropy matrix file from the daemon.
   * @param {number} [lineCount=400000] — Number of lines to generate.
   * @param {string} [entropyPath=''] — Custom path for the entropy file.
   * @returns {Promise<Object>} Generation result with key_hex, coordinates, etc.
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
   * Register a handler for generation progress updates.
   * @param {Function} handler — Callback receiving {progress, stage, message}.
   * @returns {Function} Unsubscribe function.
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
   * Register a handler for generation results.
   * Also resolves the pending generateMatrix() promise.
   * @param {Function} handler — Callback receiving the result object.
   * @returns {Function} Unsubscribe function.
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
   * Register a handler for error messages from the daemon.
   * Also rejects the pending generateMatrix() promise if active.
   * @param {Function} handler — Callback receiving the error message string.
   * @returns {Function} Unsubscribe function.
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

  // ─── Security Operations ─────────────────────────────────────────────────

  /**
   * Send a zeroize confirmation to the daemon.
   * Used to confirm secure memory wipe after a panic wipe.
   */
  zeroizeConfirm() {
    this._send(MSG_ZEROIZE_CONFIRM)
  }

  /**
   * Initiate a panic wipe — securely destroys all vault data files.
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
    * Retrieve the encrypted recovery phrase using the master password.
    * If the daemon returns SKE-encrypted data, decrypt locally.
    * @param {string} password — Master password to decrypt the stored phrase.
    * @returns {Promise<string>} The decrypted 200-character recovery phrase.
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

  // ─── Log & Diagnostics ───────────────────────────────────────────────────

  /**
   * Register a handler for broadcast log messages from the daemon.
   * @param {Function} handler — Callback receiving the log text string.
   * @returns {Function} Unsubscribe function.
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
   * Request the security header from the daemon (failed attempts, lockdown info).
   * Fires a one-shot request; the response arrives via MSG_HEADER handler.
   */
  requestHeader() {
    this._send(MSG_GET_HEADER)
  }

  /**
   * Parse a 26-byte binary header payload into a structured object.
   * @param {Uint8Array} payload — Raw 26-byte header data.
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

  // ─── Workspace Management ─────────────────────────────────────────────────

  /**
   * List all workspaces from the daemon.
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
   * Create a new workspace with the given name.
   * @param {string} name — Display name for the new workspace.
   * @returns {Promise<Object>} The created workspace object.
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
   * Switch to a different workspace by ID.
   * Dispatches AUTH.LOGOUT after switch to force re-login.
   * @param {string} id — UUID of the workspace to switch to.
   * @returns {Promise<Object>} The switched-to workspace object.
   */
  switchWorkspace(id) {
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
      this._send(MSG_WORKSPACE_SWITCH, payload)
    })
  }

  /**
   * Delete a workspace by ID.
   * @param {string} id — UUID of the workspace to delete.
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

  // ─── Omega+ Operations ───────────────────────────────────────────────────

  /**
   * Query vault entries by category.
   * @param {string} category — "PASSWORD"|"SSH_KEY"|"CERTIFICATE"|"FILE_VAULT"|"" (all)
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
   * Generate an Ed25519 SSH key pair and optionally save it to the vault.
   * @param {string} [comment] — Key comment (e.g. "user@host" or a label).
   * @param {boolean} [saveToVault=true] — Whether to persist the key pair in the vault.
   * @param {string} [passphrase=''] — Optional passphrase to encrypt the private key PEM.
   * @param {boolean} [autoPassphrase=false] — If true, the daemon generates a secure random passphrase.
   *   The generated passphrase is returned once in the response and never stored in the vault.
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
   * Ingest a File object into the FileVault.
   * Uses the 3-message streaming protocol: BEGIN → CHUNK(s) → END.
   * @param {File} file — The File object to ingest.
   * @param {Function} [onProgress] — Called with 0..1 progress fraction.
   * @returns {Promise<Object>} The BlobManifest returned by the daemon.
   */
  ingestFile(file, onProgress = null) {
    return new Promise((resolve, reject) => {
      if (!this.connected) {
        reject(new Error('Not connected to daemon'))
        return
      }

      const CHUNK_SIZE = 64 * 1024 // 64KB WebSocket chunks

      // Step 1: Send BEGIN
      const beginPayload = JSON.stringify({
        file_name: file.name,
        mime_type: file.type || 'application/octet-stream',
        total_size: file.size,
      })

      const ackHandler = this.on(MSG_ACK, () => {
        ackHandler()
        // Step 2: Stream chunks
        const reader = new FileReader()
        let offset = 0

        const sendNextChunk = () => {
          if (offset >= file.size) {
            // Step 3: Send END
            const endHandler = this.on(MSG_ENTRY_RESULT, (payload) => {
              endHandler()
              errHandler()
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
          reader.onerror = () => reject(new Error('Failed to read file chunk'))
        }

        sendNextChunk()
      })

      const errHandler = this.on(MSG_ERROR, (payload) => {
        ackHandler()
        errHandler()
        const error = new TextDecoder().decode(payload)
        reject(new Error(error))
      })

      const onProgressHandler = onProgress
        ? this.on(MSG_INGEST_PROGRESS, (payload) => {
            try {
              const text = new TextDecoder().decode(payload)
              const prog = JSON.parse(text)
              if (onProgressHandler) onProgress(prog.progress || 0)
            } catch (_) {}
          })
        : null

      this._send(MSG_FILE_INGEST_BEGIN, new TextEncoder().encode(beginPayload))
    })
  }

  // ─── Lifecycle ─────────────────────────────────────────────────────────────

  /**
   * Close the WebSocket connection and clean up all timers and handlers.
   * Safe to call multiple times.
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