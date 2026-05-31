package websocket

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/api/ipc"
)

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		// Only allow connections from localhost or the Tauri app scheme.
		allowed := []string{
			"http://localhost",
			"http://127.0.0.1",
			"tauri://localhost",
			"https://tauri.localhost",
			"http://tauri.localhost",
		}
		for _, a := range allowed {
			if len(origin) >= len(a) && origin[:len(a)] == a {
				return true
			}
		}
		return false
	},
}

// MessageHandler is called for every validated binary message from the frontend.
type MessageHandler func(msgType byte, payload []byte, conn *gorillaws.Conn) error

// HandshakeHandler is called immediately after a client authenticates.
type HandshakeHandler func(conn *gorillaws.Conn) error

// bufferedAuth holds an AUTH.TOKEN_SUBMIT that arrived before the kernel was ready.
type bufferedAuth struct {
	payload []byte
	conn    *gorillaws.Conn
}

// SessionClearer is called when all clients disconnect to protect the vault.
type SessionClearer func()

// Bridge authenticates WebSocket clients (token or cookie) and delegates
// messages to the registered MessageHandler.
type Bridge struct {
	token     string
	cookie    [ipc.CookieSize]byte
	handler   MessageHandler
	handshake HandshakeHandler

	mu        sync.Mutex
	clients   map[*gorillaws.Conn]bool
	connMutex map[*gorillaws.Conn]*sync.Mutex

	// Boot-up readiness
	readyMu    sync.RWMutex
	ready      bool
	authBuffer []bufferedAuth
	authBufMu  sync.Mutex

	// Heartbeat lifecycle
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc

	// Session clearing on disconnect
	sessionClearer SessionClearer
}

// NewBridge creates a Bridge.
func NewBridge(token string, cookie [ipc.CookieSize]byte, handler MessageHandler) *Bridge {
	return &Bridge{
		token:      token,
		cookie:     cookie,
		handler:    handler,
		clients:    make(map[*gorillaws.Conn]bool),
		connMutex:  make(map[*gorillaws.Conn]*sync.Mutex),
		authBuffer: make([]bufferedAuth, 0, 4),
	}
}

// SetHandshakeHandler registers a callback to emit the kernel status on connection.
func (b *Bridge) SetHandshakeHandler(h HandshakeHandler) {
	b.handshake = h
}

// SetSessionClearer registers a callback that clears the vault session when
// the last client disconnects (point #13 of the restoration plan).
func (b *Bridge) SetSessionClearer(fn SessionClearer) {
	b.sessionClearer = fn
}

// SetReady marks the bridge as ready and drains any buffered auth requests.
func (b *Bridge) SetReady() {
	b.readyMu.Lock()
	b.ready = true
	b.readyMu.Unlock()

	log.Printf("[bridge][handshake] Kernel ready — draining auth buffer (%d pending)", len(b.authBuffer))

	b.authBufMu.Lock()
	pending := make([]bufferedAuth, len(b.authBuffer))
	copy(pending, b.authBuffer)
	b.authBuffer = b.authBuffer[:0]
	b.authBufMu.Unlock()

	for _, req := range pending {
		log.Printf("[bridge][handshake] Draining buffered AUTH.TOKEN_SUBMIT from %s", req.conn.RemoteAddr().String())
		if b.handler != nil {
			_ = b.handler(ipc.MsgAuthTokenSubmit, req.payload, req.conn)
		}
	}
}

// StartHeartbeat begins broadcasting SYSTEM.HEARTBEAT to all clients at the given interval.
func (b *Bridge) StartHeartbeat(ctx context.Context, interval time.Duration) {
	b.heartbeatCtx, b.heartbeatCancel = context.WithCancel(ctx)
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-b.heartbeatCtx.Done():
				log.Printf("[bridge] Heartbeat emitter stopped")
				return
			case <-ticker.C:
				b.Broadcast(ipc.MsgSystemHeartbeat, nil)
			}
		}
	}()
}

// CloseConnection performs a thread-safe, idempotent close of a client socket
// and removes it from the client map to prevent zombie connections.
// If this was the last connected client, the session clearer is invoked.
func (b *Bridge) CloseConnection(conn *gorillaws.Conn) {
	if conn == nil {
		return
	}
	b.mu.Lock()
	if _, ok := b.clients[conn]; ok {
		delete(b.clients, conn)
	}
	delete(b.connMutex, conn)
	remaining := len(b.clients)
	b.mu.Unlock()
	conn.Close()
	log.Printf("[bridge] Connection closed and cleaned up (%d remaining)", remaining)

	_ = remaining
}

// HandleWebSocket is the HTTP handler for the /ws endpoint.
func (b *Bridge) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	tokenParam := r.URL.Query().Get("token")
	cookieParam := r.URL.Query().Get("cookie")

	log.Printf("[WS] Connection attempt from %s (token=%v cookie=%v)",
		r.RemoteAddr, tokenParam != "", cookieParam != "")

	if tokenParam != "" {
		if tokenParam != b.token {
			log.Printf("[WS] Token auth failed from %s", r.RemoteAddr)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
	} else if cookieParam != "" {
		raw, err := base64.StdEncoding.DecodeString(cookieParam)
		if err != nil || len(raw) != ipc.CookieSize {
			http.Error(w, "invalid cookie", http.StatusUnauthorized)
			return
		}
		var recv [ipc.CookieSize]byte
		copy(recv[:], raw)
		if recv != b.cookie {
			http.Error(w, "invalid cookie", http.StatusUnauthorized)
			return
		}
	} else {
		http.Error(w, "missing token or cookie", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	connMu := &sync.Mutex{}
	b.mu.Lock()
	b.clients[conn] = true
	b.connMutex[conn] = connMu
	b.mu.Unlock()

	log.Printf("[WS] Client authenticated from %s", r.RemoteAddr)

	// Hold the per-connection write mutex for the entire initial write sequence.
	// Broadcast goroutines acquire the same mutex via safeWrite, so they will
	// block until we release it — eliminating the concurrent-write race.
	connMu.Lock()

	log.Printf("[bridge][handshake] INIT.READY sent to %s", r.RemoteAddr)
	if err := WriteMessage(conn, ipc.MsgInitReady, nil); err != nil {
		connMu.Unlock()
		log.Printf("[bridge][handshake] Failed to send INIT.READY: %v", err)
		b.CloseConnection(conn)
		return
	}

	// Phase 1b: Emit legacy handshake status if handler registered.
	// The handler may call WriteMessage directly — safe because we hold connMu.
	if b.handshake != nil {
		if err := b.handshake(conn); err != nil {
			connMu.Unlock()
			log.Printf("[WS] Handshake failed: %v", err)
			b.CloseConnection(conn)
			return
		}
	}

	connMu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] WebSocket readLoop: %v — closing connection", r)
				b.CloseConnection(conn)
			}
		}()
		b.readLoop(conn)
	}()
}

func (b *Bridge) readLoop(conn *gorillaws.Conn) {
	defer func() {
		b.CloseConnection(conn)
		log.Printf("[WS] Client disconnected")
	}()

	conn.SetReadLimit(64 * 1024 * 1024)

	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return conn.WriteControl(gorillaws.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
	})

	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		return nil
	})

	conn.SetReadDeadline(time.Now().Add(90 * time.Second))

	connDone := make(chan struct{})

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-connDone:
				return
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteControl(gorillaws.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	defer close(connDone)

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			closeErr, isClose := err.(*gorillaws.CloseError)
			if isClose {
				log.Printf("[WS] Read error: websocket closed — code=%d reason=%q addr=%s",
					closeErr.Code, closeErr.Text, conn.RemoteAddr())
			} else if gorillaws.IsUnexpectedCloseError(err, gorillaws.CloseGoingAway, gorillaws.CloseNormalClosure) {
				log.Printf("[WS] Read error (unexpected): %v addr=%s", err, conn.RemoteAddr())
			}
			return
		}

		if messageType != gorillaws.BinaryMessage {
			continue
		}

		if len(data) < 5 {
			_ = b.safeWrite(conn, ipc.MsgError, []byte("message too short"))
			continue
		}

		msgLen := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
		if uint32(len(data)) < 4+msgLen {
			_ = b.safeWrite(conn, ipc.MsgError, []byte("incomplete message"))
			continue
		}

		msgType := data[4]
		payload := data[5 : 4+msgLen]

		// Buffer AUTH.TOKEN_SUBMIT if kernel is still booting
		if msgType == ipc.MsgAuthTokenSubmit {
			b.readyMu.RLock()
			isReady := b.ready
			b.readyMu.RUnlock()

			if !isReady {
				// Copy payload — the underlying buffer will be reused
				payloadCopy := make([]byte, len(payload))
				copy(payloadCopy, payload)

				b.authBufMu.Lock()
				b.authBuffer = append(b.authBuffer, bufferedAuth{payload: payloadCopy, conn: conn})
				b.authBufMu.Unlock()

				log.Printf("[bridge][handshake] AUTH.TOKEN_SUBMIT buffered from %s (kernel boot-up)", conn.RemoteAddr().String())
				continue
			}
		}

		// Hold connMu for the entire handler invocation.
		// The handler (translator) calls ws.WriteMessage directly — safe
		// because we hold connMu here. Broadcast goroutines call safeWrite
		// which also acquires connMu, so they block until we release.
		b.mu.Lock()
		connMu := b.connMutex[conn]
		b.mu.Unlock()
		if connMu != nil {
			connMu.Lock()
		}
		if err := b.handler(msgType, payload, conn); err != nil {
			_ = WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
		}
		if connMu != nil {
			connMu.Unlock()
		}
	}
}

// WriteMessage encodes and sends a length-prefixed binary frame.
// Callers that may race (Broadcast, readLoop errors, handshake) must use
// Bridge.safeWrite instead of calling this directly.
func WriteMessage(conn *gorillaws.Conn, msgType byte, payload []byte) error {
	msgLen := 1 + len(payload)
	data := make([]byte, 4+msgLen)
	data[0] = byte(msgLen >> 24)
	data[1] = byte(msgLen >> 16)
	data[2] = byte(msgLen >> 8)
	data[3] = byte(msgLen)
	data[4] = msgType
	copy(data[5:], payload)
	return conn.WriteMessage(gorillaws.BinaryMessage, data)
}

// safeWrite is the single serialized write path for a connection.
// gorilla/websocket allows at most one concurrent writer per connection.
func (b *Bridge) safeWrite(conn *gorillaws.Conn, msgType byte, payload []byte) error {
	b.mu.Lock()
	connMu := b.connMutex[conn]
	b.mu.Unlock()
	if connMu == nil {
		return WriteMessage(conn, msgType, payload)
	}
	connMu.Lock()
	defer connMu.Unlock()
	return WriteMessage(conn, msgType, payload)
}

// SafeWrite is the exported variant of safeWrite for use by goroutines
// running outside the readLoop (e.g. file-ingest progress callbacks).
// gorilla/websocket is NOT goroutine-safe: all writes from background
// goroutines MUST go through SafeWrite to avoid concurrent-write panics
// that close the connection with close 1005 (no status).
func (b *Bridge) SafeWrite(conn *gorillaws.Conn, msgType byte, payload []byte) error {
	return b.safeWrite(conn, msgType, payload)
}

// Broadcast sends a message to all connected clients.
func (b *Bridge) Broadcast(msgType byte, payload []byte) {
	b.mu.Lock()
	clients := make([]*gorillaws.Conn, 0, len(b.clients))
	for conn := range b.clients {
		clients = append(clients, conn)
	}
	b.mu.Unlock()

	for _, conn := range clients {
		go func(c *gorillaws.Conn) {
			if err := b.safeWrite(c, msgType, payload); err != nil {
				log.Printf("[websocket] Broadcast write error: %v", err)
			}
		}(conn)
	}
}

// GenerateSecureToken generates a URL-safe base64 token (32 random bytes).
func GenerateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
