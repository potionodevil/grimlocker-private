const MSG_GET_HEADER = 0x01
const MSG_HEADER = 0x02
const MSG_GET_CIPHERTEXT = 0x03
const MSG_CIPHERTEXT = 0x04
const MSG_UPDATE_HEADER = 0x05
const MSG_UPDATE_CIPHERTEXT = 0x06
const MSG_TRIGGER_WIPE = 0x07
const MSG_ACK = 0x08
const MSG_ERROR = 0x09
const MSG_PANIC_WIPE = 0x0A

const HEADER_SIZE = 26

class IpcClient {
  constructor() {
    this.socket = null
    this.cookie = null
    this.connected = false
    this.handlers = new Map()
    this.pendingRequests = new Map()
    this.requestId = 0
  }

  connect(cookie) {
    return new Promise((resolve, reject) => {
      this.cookie = cookie
      const wsUrl = `ws://${window.location.host}/ws?cookie=${encodeURIComponent(cookie)}`

      this.socket = new WebSocket(wsUrl)
      this.socket.binaryType = 'arraybuffer'

      this.socket.onopen = () => {
        this.connected = true
        this._attachMessageHandler()
        this._emit('connected')
        resolve()
      }

      this.socket.onclose = (event) => {
        this.connected = false
        this._emit('disconnected', { code: event.code, reason: event.reason })
      }

      this.socket.onerror = (error) => {
        this._emit('error', error)
        reject(error)
      }
    })
  }

  _attachMessageHandler() {
    this.socket.onmessage = (event) => {
      this._processMessage(event.data)
    }
  }

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

    this._dispatch(msgType, payload)
  }

  _dispatch(msgType, payload) {
    const handlers = this.handlers.get(msgType) || []
    handlers.forEach(fn => fn(payload))

    const broadcastHandlers = this.handlers.get('any') || []
    broadcastHandlers.forEach(fn => fn(msgType, payload))
  }

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

  _emit(event, data) {
    this._dispatch(event === 'connected' ? MSG_ACK : event === 'disconnected' ? 0xFF : 0xFE, new Uint8Array())
  }

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

  _send(msgType, payload = new Uint8Array(0)) {
    if (!this.connected || !this.socket) {
      throw new Error('IPC not connected')
    }
    const frame = this._frameMessage(msgType, payload)
    this.socket.send(frame)
  }

  requestHeader() {
    this._send(MSG_GET_HEADER)
  }

  requestCiphertext() {
    this._send(MSG_GET_CIPHERTEXT)
  }

  updateHeader(header) {
    const buf = new Uint8Array(HEADER_SIZE)
    buf[0] = header.failedAttempts
    const lockdownTs = new DataView(buf.buffer)
    lockdownTs.setBigUint64(1, BigInt(header.lockdownTimestamp), false)
    buf[9] = header.overrideAttemptsLeft
    lockdownTs.setBigUint64(10, BigInt(header.monotonicBootTicks), false)
    lockdownTs.setBigUint64(18, BigInt(header.wallclockLastSeen), false)
    this._send(MSG_UPDATE_HEADER, buf)
  }

  updateCiphertext(ciphertext) {
    this._send(MSG_UPDATE_CIPHERTEXT, new Uint8Array(ciphertext))
  }

  triggerWipe() {
    this._send(MSG_TRIGGER_WIPE)
  }

  triggerPanicWipe() {
    this._send(MSG_PANIC_WIPE)
  }

  parseHeader(payload) {
    if (payload.length !== HEADER_SIZE) {
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

  disconnect() {
    if (this.socket) {
      this.socket.close()
      this.socket = null
      this.connected = false
    }
  }
}

export const ipc = new IpcClient()
export { MSG_HEADER, MSG_CIPHERTEXT, MSG_ERROR, MSG_ACK, MSG_PANIC_WIPE, HEADER_SIZE }
