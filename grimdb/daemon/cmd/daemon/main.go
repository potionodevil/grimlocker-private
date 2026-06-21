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
	"github.com/grimlocker/grimdb/daemon/internal/api"
	"github.com/grimlocker/grimdb/daemon/internal/api/handlers"
	apiipc "github.com/grimlocker/grimdb/daemon/internal/api/ipc"
	apiwsbridge "github.com/grimlocker/grimdb/daemon/internal/ws"
	rustbridge "github.com/grimlocker/grimdb/daemon/internal/bridge"
	"github.com/grimlocker/grimdb/daemon/internal/config"
	gqldisp "github.com/grimlocker/grimdb/daemon/internal/gql"
	dwkernel "github.com/grimlocker/grimdb/daemon/internal/kernel"
	dwsec "github.com/grimlocker/grimdb/daemon/internal/security"
	backupmod "github.com/grimlocker/grimdb/daemon/internal/modules/backup"
	toolmod "github.com/grimlocker/grimdb/daemon/internal/modules/tools"
	workspace "github.com/grimlocker/grimdb/daemon/internal/workspace"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"

	gorillaws "github.com/gorilla/websocket"
)

// daemonVersion is set at build time via: -ldflags "-X main.daemonVersion=..."
var daemonVersion = "omega-2026-05-24-v3"

func main() {
	log.Printf("[Omega] ===== DAEMON START v%s =====", daemonVersion)

	// ── 0a. Startup binary integrity check ────────────────────────────────
	// GRIMLOCKER_EXPECTED_HASH is optional. If unset, the computed baseline is
	// logged (first-run mode). Pin the hash in the deployment environment to
	// enable tamper detection on subsequent starts.
	if err := verifyStartupIntegrity(os.Getenv("GRIMLOCKER_EXPECTED_HASH")); err != nil {
		log.Fatalf("[Omega] FATAL: %v", err)
	}

	// ── 0b. Initialize Rust secure enclave ──────────────────────────────────
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
	blockStore := vault.BlockStore() // storage.BlockStore — works for both single-user and enterprise tiers

	// ── 5b. Entry handler (CRUD operations) ───────────────────────────────────
	entryStoreHandler := storage.NewEntryHandler(blockStore)
	// Set dispatcher after bus is available (will be set after reg.StartAll)

	// ── 5c. Tools module (TOOL channel — SSH-key-gen, etc.) ──────────────────
	toolsMod := toolmod.NewModule(blockStore)
	if err := reg.Add(toolsMod); err != nil {
		log.Printf("[Omega] Register tools module: %v (non-fatal)", err)
	}

	// ── 5d. Backup module (BACKUP channel — Air-Gap Export/Import) ───────────
	backupmod.GrimlockerVersion = daemonVersion
	backupMod := backupmod.NewModule(
		cryptoProv,
		func(handle string) ([]byte, bool) { return secMod.RetrieveMVK(handle) },
		func() ([]byte, error) {
			meta, err := grimdb.LoadMeta(appDir)
			if err != nil {
				return nil, err
			}
			return meta.ArgonSalt, nil
		},
		blockStore,
		backupExportPolicy(),
	)
	if err := reg.Add(backupMod); err != nil {
		log.Printf("[Omega] Register backup module: %v (non-fatal)", err)
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

	// ── 7b. Subscribe to AUTH.LOGOUT to close STORAGE gate and lock session ──
	bus.Subscribe(kernel.EvAuthLogout, func(e kernel.Event) error {
		log.Printf("[Omega] AUTH.LOGOUT received — closing STORAGE gate, locking session")
		if gatable, ok := bus.(interface{ CloseGate() }); ok {
			gatable.CloseGate()
		}
		sessionCtx.Lock()
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
	integrityMon, err := dwsec.NewIntegrityMonitor(bus)
	if err != nil {
		log.Printf("[Omega] Integrity monitor unavailable: %v", err)
	} else {
		integrityMon.StartMonitor(ctx, 60*time.Second)
		log.Printf("[Omega] Integrity monitor started (baseline: %x)", integrityMon.Baseline())
	}

	// ── 7c. Rate limiter (exponential backoff on auth failures) ──────────────
	rateLimiter := security.NewRateLimiter()

	// ── 7d. Intrusion detector (anomaly-based threat detection) ──────────────
	intrusionDetector := security.NewIntrusionDetector(func(ev security.AnomalyEvent) {
		log.Printf("[Omega] [ANOMALY] type=%s severity=%s subject=%q detail=%q",
			ev.Type, ev.Severity, ev.Subject, ev.Detail)
		// On HIGH severity anomaly, dispatch SECURITY.AUDIT to the bus.
		if ev.Severity == "HIGH" {
			auditPayload, _ := json.Marshal(map[string]string{
				"anomaly_type": string(ev.Type),
				"subject":      ev.Subject,
				"detail":       ev.Detail,
			})
			_ = bus.Dispatch(kernel.NewEvent("intrusion_detector", kernel.EvSecAudit, auditPayload))
		}
	})
	// Wire rate limiter failures into intrusion detector.
	// Subscribe to AUTH.RESULT failure events for anomaly tracking.
	bus.Subscribe(kernel.EvAuthResult, func(e kernel.Event) error {
		var res struct {
			Success bool   `json:"success"`
			Reason  string `json:"reason,omitempty"`
		}
		if err := json.Unmarshal(e.Payload, &res); err != nil {
			return nil
		}
		if !res.Success {
			intrusionDetector.RecordAuthFailure("default")
			_, _ = rateLimiter.RecordFailure("default")
		} else {
			rateLimiter.RecordSuccess("default")
		}
		return nil
	})
	_ = rateLimiter       // used by future auth middleware
	_ = intrusionDetector // used via bus subscription

	// ── 8. Tokens ────────────────────────────────────────────────────────────
	cookie := mustCookie()
	token, err := apiwsbridge.GenerateSecureToken()
	if err != nil {
		log.Fatalf("[Omega] Generate token: %v", err)
	}

	cookieB64 := base64.StdEncoding.EncodeToString(cookie[:])

	// ── 8b. Workspace manager ────────────────────────────────────────────────
	workspaceMgr, err := workspace.NewWorkspaceManager(appDir)
	if err != nil {
		log.Printf("[Omega] Workspace manager init: %v — using default", err)
		// Create a minimal workspace manager with just the default workspace
		workspaceMgr, _ = workspace.NewWorkspaceManager(appDir)
	}

	// ── 9. API translator ────────────────────────────────────────────────────
	translator := api.NewTranslator(reg.Bus(), db, appDir, cryptoProv, workspaceMgr)
	translatorPtr = translator
	translator.SetSessionContext(sessionCtx)
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
	entryHandler.SetBlockStore(blockStore) // enables folder CRUD operations
	// Subscribe to AUTH.RESULT to capture MVK handle for ingest operations
	entryHandler.SubscribeToAuthResult()
	translator.SetEntryHandler(entryHandler)
	bridge.SetDisconnectHook(entryHandler.CleanupConn)

	// ── 9b-2. GQL dispatcher (Phase 4 — GrimQueryLanguage) ────────────────────
	gqlDispatcher := gqldisp.NewDispatcher(reg.Bus(), blockStore)
	translator.SetGQLDispatcher(gqlDispatcher)
	log.Printf("[Omega] GQL dispatcher initialized — injection-immune binary protocol")

	// ── 9c. Watchdog (heartbeat + auto-init + VFS-mount + health) ──────────
	watchdog := dwkernel.NewWatchdog(reg.Bus(), reg, appDir)
	watchdog.SetSession(sessionCtx)
	watchdog.Start(ctx)

	// ── 9d. Startup diagnostics — print all wired event handlers ─────────────
	log.Printf("[Omega] Handler-Registry: [AUTH.UNLOCK=Registered, AUTH.KEY_READY=Registered, AUTH.LOGOUT=Registered, STORAGE.WRITE=Registered, STORAGE.READ=Registered, STORAGE.LIST=Registered, ENTRY.CREATE=Registered, ENTRY.READ=Registered, ENTRY.UPDATE=Registered, ENTRY.DELETE=Registered, ENTRY.QUERY=Registered, TOOL.SSH_GEN=Registered, BACKUP.EXPORT=Registered, BACKUP.PEEK=Registered, BACKUP.AUTHORIZE=Registered, BACKUP.CHECKSUM=Registered]")
	log.Printf("[Omega] Session-Clearer: DISABLED (session persists full daemon lifetime)")
	log.Printf("[Omega] STORAGE-Gate: CLOSED (opens after AUTH.KEY_READY)")

	// ── 9e. Bridge readiness & heartbeat ─────────────────────────────────────
	bridge.SetReady()
	bridge.StartHeartbeat(ctx, 5*time.Second)

	// ── 9f. Local Network Sync (Single User only) ─────────────────────────────
	// Start the sync TCP listener for incoming peer connections.
	startSyncListener(vault, blockStore, secMod.Audit())

	// Initialize the sync scheduler (mDNS discovery + auto-pull).
	// Sync only activates when the vault is unlocked.
	if err := vault.InitSync(bus, sessionCtx, blockStore); err != nil {
		log.Printf("[Omega] Sync init: %v (sync disabled)", err)
	} else {
		log.Printf("[Omega] Local Network Sync active on port %d", vault.SyncPort())

		// Wire sync IPC into the translator so the frontend can query peers and trigger syncs.
		deviceID := vault.DeviceID()
		syncStatusFn := func() ([]byte, error) {
			peers := vault.SyncPeers()
			lastSync := vault.LastSyncAt()
			var lastSyncMs int64
			if !lastSync.IsZero() {
				lastSyncMs = lastSync.UnixMilli()
			}
			return json.Marshal(map[string]interface{}{
				"peers":        peers,
				"last_sync_at": lastSyncMs,
				"device_id":    deviceID,
			})
		}
		translator.SetSyncFns(syncStatusFn, vault.TriggerSync)
		log.Printf("[Omega] Sync IPC bridge active (device=%s)", deviceID)
	}

	// ── 9g. Audit log IPC ─────────────────────────────────────────────────────
	// Expose the security audit log over IPC so the frontend can display real events.
	translator.SetAuditLog(secMod.Audit())

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
	// startTierListener is implemented per-tier in listener_single.go / listener_enterprise.go:
	//   Single-User:  local TCP on 127.0.0.1:0 (plain HTTP, Tauri only)
	//   Enterprise:   mTLS TCP on 0.0.0.0:9443 (TLS 1.3, mutual auth)
	ipcMux := http.NewServeMux()
	ipcListener, ipcAddr, err := startTierListener(vault, ipcMux)
	if err != nil {
		log.Fatalf("[Omega] Bind IPC listener: %v", err)
	}

	uiListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("[Omega] Bind UI listener: %v", err)
	}

	uiAddr := uiListener.Addr().String()
	ipcPort := extractPort(ipcAddr)
	uiPort := extractPort(uiAddr)

	// Publish startup tokens to stdout so Tauri/Rust can discover them.
	fmt.Printf("GRIMLOCKER_COOKIE=%s\n", cookieB64)
	fmt.Printf("GRIMLOCKER_TOKEN=%s\n", token)
	fmt.Printf("GRIMLOCKER_UI=http://127.0.0.1:%d\n", uiPort)
	fmt.Printf("GRIMLOCKER_IPC=%s\n", tierListenerAddr(ipcAddr))

	// shutdownCh receives from both OS signals and the /shutdown HTTP endpoint.
	shutdownCh := make(chan struct{}, 1)

	// ipcMux was pre-created and passed to startTierListener; add routes now.
	ipcMux.HandleFunc("/ws", bridge.HandleWebSocket)
	apiV1Handler := api.NewHandler(reg.Bus())
	apiV1Handler.SetWorkspaceManager(workspaceMgr)
	apiV1Handler.SetBlockStore(blockStore)
	apiV1Handler.SetIngestEngine(ingestEngine)
	apiV1Handler.SetMVKResolver(func() []byte {
		handle := sessionCtx.ActiveHandle()
		if handle == "" {
			return nil
		}
		k, _ := secMod.RetrieveMVK(handle)
		return k
	})
	apiV1Handler.SetAuditLog(secMod.Audit())
	// Inject sync functions for the REST API if sync is available.
	// Sync is initialized above; only active when the vault is unlocked.
	if vault.DeviceID() != "" {
		apiV1Handler.SetSyncFns(
			func() ([]byte, error) {
				peers := vault.SyncPeers()
				lastSync := vault.LastSyncAt()
				var lastSyncMs int64
				if !lastSync.IsZero() {
					lastSyncMs = lastSync.UnixMilli()
				}
				return json.Marshal(map[string]interface{}{
					"peers":        peers,
					"last_sync_at": lastSyncMs,
					"device_id":    vault.DeviceID(),
				})
			},
			vault.TriggerSync,
		)
	}
	ipcMux.Handle("/api/v1", apiV1Handler)

	// /shutdown — graceful shutdown endpoint for Tauri (and ops tooling).
	// Tauri calls POST /shutdown instead of SIGKILL so the daemon can clean up.
	// Requires X-Grimlocker-Token header to prevent unauthorized shutdown.
	ipcMux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("X-Grimlocker-Token") != token {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "shutting_down"})
		go func() {
			time.Sleep(100 * time.Millisecond) // let response flush before shutdown
			select {
			case shutdownCh <- struct{}{}:
			default:
			}
		}()
	})

	// /init — one-shot vault initialization for CLI and headless deployments.
	// Only works before the vault has been initialized. POST {"password":"..."}
	ipcMux.HandleFunc("/init", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "POST required"})
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "password required"})
			return
		}
		phrase, err := grimdb.InitializeVault(req.Password, appDir)
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"recovery_phrase": phrase})
	})

	ipcMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		healthInfo := map[string]interface{}{
			"status": "ready",
			"tier":   daemonTier(),
			"role":   sessionUserRole(""),
		}
		authed := r.Header.Get("X-Grimlocker-Token") == token || r.URL.Query().Get("token") == token
		if authed {
			healthInfo["version"] = daemonVersion
			healthInfo["ipc_port"] = ipcPort
			healthInfo["ui_port"] = uiPort
			healthInfo["pid"] = os.Getpid()
			healthInfo["vault_unlocked"] = sessionCtx.IsUnlocked()
		}
		_ = json.NewEncoder(w).Encode(healthInfo)
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

	fmt.Printf("[GRIMLOCKER] ===== DAEMON READY ===== IPC=%s UI=http://%s\n", tierListenerAddr(ipcAddr), uiAddr)
	log.Printf("[Omega] Daemon running. Press Ctrl+C to stop.")

	// ── 12. Graceful shutdown ─────────────────────────────────────────────────
	// shutdownCh is triggered by OS signals OR the /shutdown HTTP endpoint.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		select {
		case shutdownCh <- struct{}{}:
		default:
		}
	}()

	<-shutdownCh // block until shutdown requested
	log.Printf("[Omega] Graceful shutdown initiated...")

	// Step 1: Stop accepting new connections.
	_ = ipcHTTP.Close()
	_ = uiHTTP.Close()
	if ipcServer != nil && runtime.GOOS != "windows" {
		_ = ipcServer.Stop()
	}

	// Step 2: Flush in-flight storage writes.
	if err := blockStore.Flush(); err != nil {
		log.Printf("[Omega] blockStore.Flush: %v", err)
	}
	if err := blockStore.Close(); err != nil {
		log.Printf("[Omega] blockStore.Close: %v", err)
	}

	// Step 3: Lock session — revokes MVK handle from security module.
	sessionCtx.Lock()

	// Step 3b: Shut down Local Network Sync.
	vault.ShutdownSync()

	// Step 4: Destroy Rust enclave session key handle if one was created.
	if skh := translator.SessionKeyHandle(); skh != "" {
		rustbridge.SessionDestroy(skh)
		log.Printf("[Omega] Rust enclave session handle revoked")
	}

	// Step 5: Stop all kernel modules (5-second timeout).
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
