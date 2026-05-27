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
	"syscall"
	"time"

	omega "github.com/grimlocker/grimdb"
	"github.com/grimlocker/grimdb/api"
	"github.com/grimlocker/grimdb/api/handlers"
	apiipc "github.com/grimlocker/grimdb/api/ipc"
	apiwsbridge "github.com/grimlocker/grimdb/api/websocket"
	rustbridge "github.com/grimlocker/grimdb/cgo"
	"github.com/grimlocker/grimdb/config"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
	"github.com/grimlocker/grimdb/storage/grimdb"
	"github.com/grimlocker/grimdb/tools"

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

	// ── 1. Storage (GrimDB file + tier config) ───────────────────────────────
	db, err := grimdb.NewGrimDB(dbPath)
	if err != nil {
		log.Fatalf("[Omega] Failed to open database: %v", err)
	}

	// ── 1b. Tier Provider (Single-User default; Enterprise via -tags enterprise) ─
	cfg := config.ConfigFromEnv(appDir)
	log.Printf("[Omega] Tier: %s", cfg.Mode)
	vault := config.NewSingleUserProvider(cfg, db)

	// ── 2. Kernel (with STORAGE gate until vault is unlocked) ─────────────────
	bus := kernel.NewBus(kernel.WithGatedChannels("STORAGE"))
	reg := kernel.NewRegistry(bus)

	// ── 2b. Session Context (global vault-unlock state) ─────────────────────
	sessionCtx := security.NewSessionContext()

	// Wire session context into the vault provider (security module + storage adapter).
	vault.SetSession(sessionCtx)

	// ── 3–5. Register all kernel modules from the vault provider ─────────────
	// Order: security → crypto → storage adapter (as returned by KernelModules).
	for _, mod := range vault.KernelModules() {
		if err := reg.Add(mod); err != nil {
			log.Fatalf("[Omega] Register module %s: %v", mod.ID(), err)
		}
	}

	// Convenience aliases for components that still need direct access.
	secMod := vault.SecurityModule()
	cryptoProv := vault.CryptoProvider()
	blockStore := vault.RawStorage().BlockStore()

	// ── 5b. Entry handler (CRUD operations) ───────────────────────────────────
	entryStoreHandler := storage.NewEntryHandler(blockStore)
	// Set dispatcher after bus is available (will be set after reg.StartAll)

	// ── 5c. Tools module (TOOL channel — SSH-key-gen, etc.) ──────────────────
	toolsMod := tools.NewModule(blockStore)
	if err := reg.Add(toolsMod); err != nil {
		log.Printf("[Omega] Register tools module: %v (non-fatal)", err)
	}

	// ── 6. Auth handler (in-bus, via provider interface) ─────────────────────
	// vault.Auth().HandleUnlockEvent replaces makeAuthUnlockHandler from main.go.
	// The provider encapsulates all 7 unlock steps; main.go only wires the subscription.
	var translatorPtr *api.Translator
	unlockHandler := vault.Auth().HandleUnlockEvent(bus, sessionCtx, func(sk []byte, skh string) {
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
	bus.Subscribe(kernel.EvEntryQuery, entryStoreHandler.Handle)

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
	log.Printf("[Omega] Handler-Registry: [AUTH.UNLOCK=Registered, AUTH.KEY_READY=Registered, AUTH.LOGOUT=Registered, STORAGE.WRITE=Registered, STORAGE.READ=Registered, STORAGE.LIST=Registered, ENTRY.CREATE=Registered, ENTRY.READ=Registered, ENTRY.UPDATE=Registered, ENTRY.DELETE=Registered, ENTRY.QUERY=Registered, TOOL.SSH_GEN=Registered]")
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

// makeAuthUnlockHandler and replyAuthFail have been extracted to
// config/single/auth.go as LocalAuth.HandleUnlockEvent().
// The daemon now calls vault.Auth().HandleUnlockEvent(...) instead.

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
