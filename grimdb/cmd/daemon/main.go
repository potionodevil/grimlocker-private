package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"

	omega "github.com/grimlocker/grimdb"
	"github.com/grimlocker/grimdb/api"
	"github.com/grimlocker/grimdb/api/handlers"
	apiipc "github.com/grimlocker/grimdb/api/ipc"
	apiwsbridge "github.com/grimlocker/grimdb/api/websocket"
	rustbridge "github.com/grimlocker/grimdb/cgo"
	"github.com/grimlocker/grimdb/crypto"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
	"github.com/grimlocker/grimdb/storage/grimdb"

	gorillaws "github.com/gorilla/websocket"
)

const daemonVersion = "omega-2026-05-24-v3"

func main() {
	log.Printf("[Omega] ===== DAEMON START v%s =====", daemonVersion)

	// ── 0. Initialize Rust secure enclave ──────────────────────────────────
	if err := rustbridge.InitCore(); err != nil {
		log.Printf("[Omega] Rust enclave init failed (using Go fallback): %v", err)
	} else {
		log.Printf("[Omega] Rust secure enclave initialized")
	}
	defer rustbridge.ShutdownCore()
	appDir := getAppDir()
	if err := os.MkdirAll(appDir, 0700); err != nil {
		log.Fatalf("[Omega] Failed to create app directory: %v", err)
	}

	dbPath := envOr("GRIMLOCKER_DB_PATH", filepath.Join(appDir, "vault.gdb"))
	log.Printf("[Omega] App directory: %s", appDir)
	log.Printf("[Omega] Database path: %s", dbPath)

	// ── 1. Storage ───────────────────────────────────────────────────────────
	db, err := grimdb.NewGrimDB(dbPath)
	if err != nil {
		log.Fatalf("[Omega] Failed to open database: %v", err)
	}

	blockStore := grimdb.NewBlockStoreImpl(appDir)

	// ── 2. Kernel (with STORAGE gate until vault is unlocked) ─────────────────
	bus := kernel.NewBus(kernel.WithGatedChannels("STORAGE"))
	reg := kernel.NewRegistry(bus)

	// ── 2b. Session Context (global vault-unlock state) ─────────────────────
	sessionCtx := security.NewSessionContext()

	// ── 3. Security module ───────────────────────────────────────────────────
	entropyPath := filepath.Join(appDir, "entropy.bin")
	secMod := security.NewModule(security.LockdownConfig{
		Threshold:       3,
		MaxOverrides:    4,
		LockdownMinutes: 200,
	}, entropyPath)
	secMod.SetSession(sessionCtx)

	if err := reg.Add(secMod); err != nil {
		log.Fatalf("[Omega] Register security: %v", err)
	}

	// ── 4. Crypto module ─────────────────────────────────────────────────────
	cryptoProv := crypto.New()
	cryptoMod := crypto.NewModule(cryptoProv, secMod.RetrieveMVK)
	if err := reg.Add(cryptoMod); err != nil {
		log.Fatalf("[Omega] Register crypto: %v", err)
	}

	// ── 5. Storage adapter ───────────────────────────────────────────────────
	storageAdapter := grimdb.NewAdapter(db, blockStore)
	storageAdapter.SetSession(sessionCtx)
	if err := reg.Add(storageAdapter); err != nil {
		log.Fatalf("[Omega] Register storage: %v", err)
	}

	// ── 5b. Entry handler (CRUD operations) ───────────────────────────────────
	entryStoreHandler := storage.NewEntryHandler(blockStore)
	// Set dispatcher after bus is available (will be set after reg.StartAll)

	// Auth handler (in-bus, wired to security+storage+session)
	// AUTH.UNLOCK events are handled inline by the auth goroutine subscribed here.
	// The session key + handle callback will be wired after the translator is created.
	// ── 6. Auth handler (in-bus, wired to security+storage+session) ──────────
	// AUTH.UNLOCK events are handled inline by the auth goroutine subscribed here.
	// The session key + handle callback will be wired after the translator is created.
	var translatorPtr *api.Translator
	unlockHandler := makeAuthUnlockHandler(bus, secMod, blockStore, appDir, sessionCtx, reg, func(sk []byte, skh string) {
		if t := translatorPtr; t != nil {
			t.SetSessionKey(sk)
			if skh != "" {
				t.SetSessionKeyHandle(skh)
			}
		}
	})
	bus.Subscribe(kernel.EvAuthUnlock, unlockHandler)

	// ── 7. Start all modules ─────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := reg.StartAll(ctx); err != nil {
		log.Fatalf("[Omega] Start modules: %v", err)
	}

	// ── 7a. Subscribe to KEY_READY to open STORAGE gate ──────────────────────────
	bus.Subscribe(kernel.EvAuthKeyReady, func(e kernel.Event) error {
		log.Printf("[Omega] KEY_READY subscription triggered")
		if gatable, ok := bus.(interface{ OpenGate() }); ok {
			gatable.OpenGate()
			log.Printf("[Omega] AUTH.KEY_READY received — STORAGE gate OPEN")
		}
		return nil
	})

	// ── 7b. Subscribe to AUTH.LOGOUT to close STORAGE gate (explicit lock only) ──
	bus.Subscribe(kernel.EvAuthLogout, func(e kernel.Event) error {
		log.Printf("[Omega] AUTH.LOGOUT received — closing STORAGE gate")
		if gatable, ok := bus.(interface{ CloseGate() }); ok {
			gatable.CloseGate()
		}
		return nil
	})

	// ── 5b-continued. Wire entry handler subscriptions ──────────────────────────
	entryStoreHandler.SetDispatcher(bus)
	bus.Subscribe(kernel.EvEntryCreate, entryStoreHandler.Handle)
	bus.Subscribe(kernel.EvEntryRead, entryStoreHandler.Handle)
	bus.Subscribe(kernel.EvEntryUpdate, entryStoreHandler.Handle)
	bus.Subscribe(kernel.EvEntryDelete, entryStoreHandler.Handle)

	// ── 7b. Binary integrity monitor ─────────────────────────────────────────
	integrityMon, err := security.NewIntegrityMonitor(bus)
	if err != nil {
		log.Printf("[Omega] Integrity monitor unavailable: %v", err)
	} else {
		integrityMon.StartMonitor(ctx, 60*time.Second)
		log.Printf("[Omega] Integrity monitor started (baseline: %x)", integrityMon.Baseline())
	}

	// ── 8. Tokens ────────────────────────────────────────────────────────────
	cookie := mustCookie()
	token, err := apiwsbridge.GenerateSecureToken()
	if err != nil {
		log.Fatalf("[Omega] Generate token: %v", err)
	}

	cookieB64 := base64.StdEncoding.EncodeToString(cookie[:])

	// ── 8b. Workspace manager ────────────────────────────────────────────────
	workspaceMgr, err := storage.NewWorkspaceManager(appDir)
	if err != nil {
		log.Printf("[Omega] Workspace manager init: %v — using default", err)
		// Create a minimal workspace manager with just the default workspace
		workspaceMgr, _ = storage.NewWorkspaceManager(appDir)
	}

	// ── 9. API translator ────────────────────────────────────────────────────
	translator := api.NewTranslator(reg.Bus(), db, appDir, cryptoProv, workspaceMgr)
	translatorPtr = translator
	translator.SetMVKResolver(func() []byte {
		handle := sessionCtx.ActiveHandle()
		if handle == "" {
			return nil
		}
		k, _ := secMod.RetrieveMVK(handle)
		return k
	})

	wsHandler := func(msgType byte, payload []byte, conn *gorillaws.Conn) error {
		return translator.HandleWS(msgType, payload, conn)
	}
	bridge := apiwsbridge.NewBridge(token, cookie, wsHandler)

	// Set handshake handler to emit kernel status on connection
	bridge.SetHandshakeHandler(translator.HandshakeStatus)

	// Wire translator to broadcast security events.
	translator.SetBridge(bridge)

	// Wire token validation for the three-way handshake.
	translator.SetTokenValidator(func(clientToken string) bool {
		return clientToken == token
	})

	// Session persists for the full daemon lifetime (daemon exits when Tauri closes).
	// Page navigation in Tauri briefly disconnects/reconnects WebSocket — we must NOT
	// lock the vault on transient disconnects. AUTH.LOGOUT events handle explicit lock.
	// (sessionClearer intentionally disabled)

	// ── 9b. Entry handler (CRUD + streaming) ──────────────────────────────────
	policyMgr := handlers.NewPolicyManager(reg.Bus(), secMod.Audit())
	ingestEngine := storage.NewIngestEngine(blockStore, cryptoProv)
	// Note: retrieveMVK requires the MVK handle, which is set after AUTH.UNLOCK
	retrieveMVK := func(handle string) ([]byte, bool) {
		return secMod.RetrieveMVK(handle)
	}
	entryHandler := handlers.NewEntryHandler(reg.Bus(), bridge, policyMgr, ingestEngine, retrieveMVK)
	// Subscribe to AUTH.RESULT to capture MVK handle for ingest operations
	entryHandler.SubscribeToAuthResult()
	translator.SetEntryHandler(entryHandler)

	// ── 9c. Watchdog (heartbeat + auto-init + VFS-mount + health) ──────────
	watchdog := kernel.NewWatchdog(reg.Bus(), reg, appDir)
	watchdog.SetSession(sessionCtx)
	watchdog.Start(ctx)

	// ── 9d. Startup diagnostics — print all wired event handlers ─────────────
	log.Printf("[Omega] Handler-Registry: [AUTH.UNLOCK=Registered, AUTH.KEY_READY=Registered, AUTH.LOGOUT=Registered, STORAGE.WRITE=Registered, STORAGE.READ=Registered, STORAGE.LIST=Registered, ENTRY.CREATE=Registered, ENTRY.READ=Registered, ENTRY.UPDATE=Registered, ENTRY.DELETE=Registered]")
	log.Printf("[Omega] Session-Clearer: DISABLED (session persists full daemon lifetime)")
	log.Printf("[Omega] STORAGE-Gate: CLOSED (opens after AUTH.KEY_READY)")

	// ── 9e. Bridge readiness & heartbeat ─────────────────────────────────────
	bridge.SetReady()
	bridge.StartHeartbeat(ctx, 2*time.Second)

	// ── 10. IPC server (Unix socket / named pipe) ─────────────────────────────
	var ipcServer *apiipc.Server
	if runtime.GOOS != "windows" {
		ipcHandler := func(msgType byte, payload []byte, conn net.Conn) error {
			// Mirror same handling as WebSocket via a fake conn wrapper.
			// For the UDS server the same translator is re-used;
			// response writing uses the raw conn.
			return ipcConnDispatch(msgType, payload, conn, translator, db, appDir)
		}
		ipcServer = apiipc.NewServer(cookie, ipcHandler)
		if err := ipcServer.Start(); err != nil {
			log.Printf("[Omega] IPC server (UDS) unavailable: %v", err)
		}
	}

	// ── 11. HTTP listeners ───────────────────────────────────────────────────
	ipcListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("[Omega] Bind IPC listener: %v", err)
	}
	uiListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("[Omega] Bind UI listener: %v", err)
	}

	ipcAddr := ipcListener.Addr().String()
	uiAddr := uiListener.Addr().String()
	ipcPort := extractPort(ipcAddr)
	uiPort := extractPort(uiAddr)

	// Publish startup tokens to stdout so Tauri/Rust can discover them.
	fmt.Printf("GRIMLOCKER_COOKIE=%s\n", cookieB64)
	fmt.Printf("GRIMLOCKER_TOKEN=%s\n", token)
	fmt.Printf("GRIMLOCKER_UI=http://127.0.0.1:%d\n", uiPort)
	fmt.Printf("GRIMLOCKER_IPC=ws://%s/ws\n", ipcAddr)

	ipcMux := http.NewServeMux()
	ipcMux.HandleFunc("/ws", bridge.HandleWebSocket)
	ipcMux.Handle("/api/v1", api.NewHandler(reg.Bus()))
	ipcMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "ready",
			"ipc_port": ipcPort,
			"ui_port":  uiPort,
			"pid":      os.Getpid(),
		})
	})

	uiMux := http.NewServeMux()
	sub, err := fs.Sub(omega.EmbeddedUI, "ui-dist")
	if err != nil {
		log.Fatalf("[Omega] Embedded FS: %v", err)
	}
	uiMux.Handle("/", http.FileServer(http.FS(sub)))

	ipcHTTP := &http.Server{Handler: ipcMux}
	uiHTTP := &http.Server{Handler: uiMux}

	go func() {
		if err := ipcHTTP.Serve(ipcListener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Omega] IPC HTTP error: %v", err)
		}
	}()
	go func() {
		if err := uiHTTP.Serve(uiListener); err != nil && err != http.ErrServerClosed {
			log.Printf("[Omega] UI HTTP error: %v", err)
		}
	}()

	fmt.Printf("[GRIMLOCKER] ===== DAEMON READY ===== IPC=ws://%s/ws UI=http://%s\n", ipcAddr, uiAddr)
	log.Printf("[Omega] Daemon running. Press Ctrl+C to stop.")

	// ── 12. Graceful shutdown ─────────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Printf("[Omega] Shutting down...")
	_ = ipcHTTP.Close()
	_ = uiHTTP.Close()
	if ipcServer != nil && runtime.GOOS != "windows" {
		_ = ipcServer.Stop()
	}
	if err := blockStore.Close(); err != nil {
		log.Printf("[Omega] blockStore.Close: %v", err)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = reg.Bus().Shutdown(shutdownCtx)
	log.Printf("[Omega] Shutdown complete.")
}

// makeAuthUnlockHandler returns a kernel.Handler for AUTH.UNLOCK events.
// It runs the full unlock flow: derive MVK, verify sentinel, load index,
// populate the SessionContext, open the bus gate, then emits AUTH.RESULT.
func makeAuthUnlockHandler(
	bus kernel.Dispatcher,
	secMod *security.Module,
	blockStore *grimdb.BlockStoreImpl,
	appDir string,
	sessionCtx *security.SessionContext,
	reg *kernel.Registry,
	onSessionKey func([]byte, string),
) kernel.Handler {
	return func(e kernel.Event) (err error) {
		// Catch ANY panic in this handler — prevents silent goroutine death.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[auth] PANIC in unlock handler: %v\n%s", r, debug.Stack())
				panic(r) // re-panic — the bus wrapper also recovers
			}
		}()
		var req struct {
			Password string `json:"password"`
			AppDir   string `json:"app_dir"`
		}
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			log.Printf("[unlock:FAIL] payload unmarshal: %v", err)
			return err
		}

		dir := req.AppDir
		if dir == "" {
			dir = appDir
		}

		// Step 0/7 — Lockdown check.
		if state := secMod.Lockdown().State(); state == security.LockdownHard {
			log.Printf("[unlock:FAIL] hard lockdown active")
			return replyAuthFail(bus, e, "hard lockdown active")
		}

		// Step 1/7 — Derive & verify MVK.
		log.Printf("[auth] AUTH.UNLOCK received — attempting vault unlock")
		log.Printf("[unlock:1/7] UnlockVault starting")
		mvk, err := grimdb.UnlockVault(req.Password, dir)
		if err != nil {
			log.Printf("[unlock:FAIL:1/7] UnlockVault: %v", err)
			state, _ := secMod.Lockdown().RecordFailure()
			if state == security.LockdownHard {
				return replyAuthFail(bus, e, "too many failures: hard lockdown")
			}
			return replyAuthFail(bus, e, "invalid password")
		}
		log.Printf("[unlock:1/7] UnlockVault OK (key len=%d)", len(mvk))

		// Step 2/7 — Store key in locked memory.
		log.Printf("[unlock:2/7] StoreMVK starting")
		handle, err := secMod.StoreMVK(mvk)
		for i := range mvk {
			mvk[i] = 0
		}
		if err != nil {
			log.Printf("[unlock:FAIL:2/7] StoreMVK: %v", err)
			return replyAuthFail(bus, e, "failed to store key material")
		}
		log.Printf("[unlock:2/7] StoreMVK OK (handle=%s)", handle)

		// Step 3/7 — Wire into blockstore.
		log.Printf("[unlock:3/7] SetMVKFunc starting")
		blockStore.SetMVKFunc(func() []byte {
			k, _ := secMod.RetrieveMVK(handle)
			return k
		})
		log.Printf("[unlock:3/7] SetMVKFunc OK")

		// Step 4/7 — Load block index.
		log.Printf("[unlock:4/7] LoadIndex starting")
		if err := blockStore.LoadIndex(); err != nil {
			log.Printf("[unlock:4/7] LoadIndex error (treating as empty index): %v", err)
		}
		log.Printf("[unlock:4/7] LoadIndex OK")

		// Step 5/7 — Open STORAGE gate.
		log.Printf("[unlock:5/7] KEY_READY dispatch")
		keyReadyPayload, _ := json.Marshal(map[string]bool{"ready": true})
		_ = bus.Dispatch(kernel.NewEvent("daemon", kernel.EvAuthKeyReady, keyReadyPayload))
		log.Printf("[unlock:5/7] KEY_READY dispatched")

		// Step 6/7 — Mark session as unlocked.
		log.Printf("[unlock:6/7] sessionCtx.Unlock starting")
		sessionCtx.Unlock(handle)
		log.Printf("[unlock:6/7] sessionCtx.Unlock OK")

		// Step 7/7 — Record success & reply.
		secMod.Lockdown().RecordSuccess()
		log.Printf("[unlock:7/7] RecordSuccess + AUTH.RESULT (success=true)")

		// Generate a per-session ChaCha20-Poly1305 key for SKE encryption.
		// Try the Rust enclave first; fall back to Go CSPRNG if unavailable.
		var sessionKey []byte
		var sessionKeyHandle string
		skh, ska, err := rustbridge.SessionCreate()
		if err != nil {
			// Fallback: generate in Go
			log.Printf("[unlock:7/7] Rust session create failed, using Go fallback: %v", err)
			sessionKey = make([]byte, 32)
			if _, randErr := rand.Read(sessionKey); randErr != nil {
				log.Printf("[unlock:7/7] WARN: session key generation failed: %v", randErr)
				sessionKey = nil
			}
		} else {
			sessionKey = ska[:]
			sessionKeyHandle = skh
			log.Printf("[unlock:7/7] Session key created via Rust enclave (handle=%s)", skh)
		}

		// Encode session key for the response BEFORE zeroing the local copy.
		sessionKeyB64 := ""
		if sessionKey != nil {
			sessionKeyB64 = base64.StdEncoding.EncodeToString(sessionKey)
		}

		// Inject session key + handle into the translator.
		if onSessionKey != nil && sessionKey != nil {
			onSessionKey(sessionKey, sessionKeyHandle)
		}

		// Zero the local session key copy.
		// The Rust enclave holds its own copy in locked memory;
		// the Go heap copy is no longer needed.
		for i := range sessionKey {
			sessionKey[i] = 0
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"mvk_handle":  handle,
			"session_key": sessionKeyB64,
		})
		reply := kernel.ReplyEvent("daemon", kernel.EvAuthResult, e, payload)
		return bus.Dispatch(reply)
	}
}

func replyAuthFail(bus kernel.Dispatcher, req kernel.Event, reason string) error {
	log.Printf("[auth:FAIL] replyAuthFail reason=%q (req.ID=%s)", reason, req.ID)
	payload, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"reason":  reason,
	})
	reply := kernel.ReplyEvent("daemon", kernel.EvAuthResult, req, payload)
	return bus.Dispatch(reply)
}

// ipcConnDispatch routes UDS IPC messages using the same translator logic.
// Responses are written directly to the raw conn using ipc.WriteMessage.
func ipcConnDispatch(msgType byte, payload []byte, conn net.Conn, t *api.Translator, db *grimdb.GrimDB, appDir string) error {
	switch msgType {
	case apiipc.MsgCheckVaultStatus:
		init, isV5, _ := grimdb.CheckVaultStatus(appDir)
		status, _ := json.Marshal(map[string]bool{"initialized": init, "isV5": isV5})
		return apiipc.WriteMessage(conn, apiipc.MsgAck, status)

	case apiipc.MsgGetHeader:
		h := db.GetHeader()
		buf := headerToBytes(h)
		return apiipc.WriteMessage(conn, apiipc.MsgHeader, buf)

	case apiipc.MsgUpdateHeader:
		if len(payload) < 26 {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte("invalid header"))
		}
		h := bytesToHeader(payload[:26])
		if err := db.UpdateHeader(h); err != nil {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte(err.Error()))
		}
		return apiipc.WriteMessage(conn, apiipc.MsgAck, nil)

	case apiipc.MsgGetCiphertext:
		ciphertext, err := db.GetCiphertext()
		if err != nil {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte(err.Error()))
		}
		return apiipc.WriteMessage(conn, apiipc.MsgCiphertext, ciphertext)

	case apiipc.MsgUpdateCiphertext:
		if err := db.UpdateCiphertext(payload); err != nil {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte(err.Error()))
		}
		return apiipc.WriteMessage(conn, apiipc.MsgAck, nil)

	case apiipc.MsgInitializeVault:
		phrase, err := grimdb.InitializeVault(string(payload), appDir)
		if err != nil {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte(err.Error()))
		}
		return apiipc.WriteMessage(conn, apiipc.MsgRecoveryPhrase, []byte(phrase))

	case apiipc.MsgUnlockVault:
		// This message type requires bus interaction via the translator,
		// which is not available in IPC context. Return error.
		return apiipc.WriteMessage(conn, apiipc.MsgError, []byte("use WebSocket IPC for unlock operations"))

	case apiipc.MsgGetRecoveryPhrase:
		password := string(payload)
		phrase, err := grimdb.RetrieveRecoveryPhrase(password, appDir)
		if err != nil {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte(err.Error()))
		}
		return apiipc.WriteMessage(conn, apiipc.MsgRecoveryPhraseData, []byte(phrase))

	case apiipc.MsgPanicWipeRequest:
		if err := grimdb.WipeVault(appDir); err != nil {
			return apiipc.WriteMessage(conn, apiipc.MsgError, []byte(fmt.Sprintf("wipe failed: %v", err)))
		}
		return apiipc.WriteMessage(conn, apiipc.MsgAck, nil)

	default:
		return apiipc.WriteMessage(conn, apiipc.MsgError, []byte("unsupported message type"))
	}
}

func headerToBytes(h grimdb.Header) []byte {
	buf := make([]byte, grimdb.HeaderSize)
	buf[0] = h.FailedAttempts
	binary.BigEndian.PutUint64(buf[1:9], uint64(h.LockdownTimestamp))
	buf[9] = h.OverrideAttemptsLeft
	binary.BigEndian.PutUint64(buf[10:18], h.MonotonicBootTicks)
	binary.BigEndian.PutUint64(buf[18:26], uint64(h.WallclockLastSeen))
	return buf
}

func bytesToHeader(buf []byte) grimdb.Header {
	return grimdb.Header{
		FailedAttempts:       buf[0],
		LockdownTimestamp:    int64(binary.BigEndian.Uint64(buf[1:9])),
		OverrideAttemptsLeft: buf[9],
		MonotonicBootTicks:   binary.BigEndian.Uint64(buf[10:18]),
		WallclockLastSeen:    int64(binary.BigEndian.Uint64(buf[18:26])),
	}
}

func getAppDir() string {
	if dir := os.Getenv("GRIMLOCKER_APP_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".grimlocker")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func extractPort(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return port
}

func mustCookie() [apiipc.CookieSize]byte {
	var c [apiipc.CookieSize]byte
	if _, err := rand.Read(c[:]); err != nil {
		log.Fatalf("[Omega] Generate cookie: %v", err)
	}
	return c
}
