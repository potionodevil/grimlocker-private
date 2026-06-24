// Package api provides the IPC bridge that translates between the binary
// wire protocol (0x01–0x1B message types) and kernel Events.
// No business logic lives here — the translator dispatches Events and
// streams results back over the connection.
package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/daemon/internal/api/handlers"
	"github.com/grimlocker/grimdb/daemon/internal/api/ipc"
	ws "github.com/grimlocker/grimdb/daemon/internal/ws"
	rustbridge "github.com/grimlocker/grimdb/daemon/internal/bridge"
	gqldisp "github.com/grimlocker/grimdb/daemon/internal/gql"
	"github.com/grimlocker/grimdb/daemon/internal/workspace"
	"github.com/grimlocker/grimdb/engine/crypto"
	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/gql"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"
	"golang.org/x/crypto/chacha20poly1305"
)

const requestTimeout = 30 * time.Second

// Translator converts binary IPC frames into kernel Events and delivers
// kernel result Events back as binary frames.
type Translator struct {
	bus           kernel.Dispatcher
	db            *grimdb.GrimDB
	appDir        string
	crypto        crypto.Provider
	bridge        *ws.Bridge
	entryHandler  *handlers.EntryHandler
	workspaceMgr  *workspace.WorkspaceManager
	gqlDispatcher *gqldisp.Dispatcher

	// tokenValidator is injected by the daemon to validate AUTH.TOKEN_SUBMIT payloads.
	tokenValidator func(string) bool

	// syncPeersFn returns JSON-encoded sync state (peers + last_sync_at + device_id).
	// Injected by main.go when Local Network Sync is available.
	syncPeersFn func() ([]byte, error)

	// syncTriggerFn fires an immediate sync cycle. Non-blocking.
	syncTriggerFn func()

	// auditLog is the security audit log, injected by main.go.
	// When set, MsgAuditList returns the last n SecurityEvents.
	auditLog security.AuditLog

	// mvkResolver returns the current master-vault key from locked memory.
	// Provided by the daemon; wraps security.Module.RetrieveMVK.
	mvkResolver func() []byte

	// sessionKey is a per-session ChaCha20-Poly1305 key generated on unlock.
	// It is used by SKE (Session Key Encryption) to encrypt sensitive data
	// before sending it over the WebSocket. The frontend holds the same key
	// in RAM and decrypts locally. Set once on unlock, zeroed on lock.
	sessionKey    []byte
	sessionKeySet bool

	// sessionKeyHandle is the Rust enclave handle for the session key (e.g. "ske:abcd1234").
	// When the Rust enclave is available, SKE encrypt/decrypt goes through the enclave
	// using this handle instead of keeping the raw key in Go's heap.
	sessionKeyHandle string

	// sessionCtx is used by HandshakeStatus to push the current vault unlock
	// state to reconnecting clients (so the UI can re-attach without re-auth).
	sessionCtx *security.SessionContext
}

// NewTranslator creates a Translator.
func NewTranslator(bus kernel.Dispatcher, db *grimdb.GrimDB, appDir string, cryptoProv crypto.Provider, workspaceMgr *workspace.WorkspaceManager) *Translator {
	return &Translator{
		bus:          bus,
		db:           db,
		appDir:       appDir,
		crypto:       cryptoProv,
		workspaceMgr: workspaceMgr,
	}
}

// SetSessionContext injects the global SessionContext so that HandshakeStatus
// can include vault_unlocked=true/false in the initial WebSocket handshake.
// This lets reconnecting clients re-attach to an already-unlocked vault without
// prompting the user for their password again.
// Call once during daemon startup, before serving WebSocket connections.
func (t *Translator) SetSessionContext(s *security.SessionContext) {
	t.sessionCtx = s
}

// SessionKeyHandle returns the Rust enclave handle for the current session key.
// Used by main.go during graceful shutdown to destroy the handle in the enclave.
func (t *Translator) SessionKeyHandle() string {
	return t.sessionKeyHandle
}

// HandshakeStatus emits the kernel state on initial WebSocket connection.
// Includes vault initialization AND unlock state so reconnecting clients
// can re-attach to an already-unlocked vault without asking the user to
// re-enter their password.
//
// Phase 3: Extended to push full vault state mirror (active workspace,
// entry count, gate status) so the UI can fully reconstruct its state
// after a transient WebSocket disconnect (e.g., Tauri page navigation).
func (t *Translator) HandshakeStatus(conn *gorillaws.Conn) error {
	initialized, _, _ := grimdb.CheckVaultStatus(t.appDir)
	unlocked := t.sessionCtx != nil && t.sessionCtx.IsUnlocked()

	state := map[string]interface{}{
		"status":      "Online",
		"initialized": initialized,
		"unlocked":    unlocked,
	}

	// Full state mirror: push additional context so the UI can re-attach
	// without re-querying the daemon for every piece of state.
	if unlocked && t.sessionCtx != nil {
		state["gate_open"] = true
		state["mvk_handle_ok"] = t.sessionCtx.MVKHandle() != ""

		// Entry count: query block store index size (non-blocking, in-memory).
		if t.bus != nil {
			ev := kernel.NewEvent("api", kernel.EvStorageList, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			result, err := t.bus.Request(ctx, ev)
			cancel()
			if err == nil {
				var stored struct {
					Metas []storage.BlockMeta `json:"metas,omitempty"`
				}
				if json.Unmarshal(result.Payload, &stored) == nil {
					state["entry_count"] = len(stored.Metas)
				}
			}
		}

		// Workspace info
		if t.workspaceMgr != nil {
			ws := t.workspaceMgr.Active()
			if ws != nil {
				state["active_workspace"] = map[string]string{
					"id":   ws.ID,
					"name": ws.Name,
				}
			}
		}
	} else {
		state["gate_open"] = false
	}

	payload, _ := json.Marshal(state)
	return ws.WriteMessage(conn, ipc.MsgAck, payload)
}

// SetEntryHandler injects the EntryHandler used by MsgEntryCreate, MsgEntryUpdate,
// MsgEntryDelete, and the file-ingest message types. Must be called after the
// entry handler's dispatcher is set (i.e. after reg.StartAll). If not set,
// those message types return an "entry handler unavailable" error to the client.
func (t *Translator) SetEntryHandler(eh *handlers.EntryHandler) {
	t.entryHandler = eh
}

// SetGQLDispatcher injects the GQL dispatcher for handling binary GQL frames.
func (t *Translator) SetGQLDispatcher(gd *gqldisp.Dispatcher) {
	t.gqlDispatcher = gd
}

// SetBridge sets the WebSocket bridge for broadcasting log events.
func (t *Translator) SetBridge(bridge *ws.Bridge) {
	t.bridge = bridge
	// Subscribe to security and auth events for logging.
	t.bus.Subscribe(kernel.EvSecPanic, t.makeLogHandler("SECURITY.PANIC"))
	t.bus.Subscribe(kernel.EvSecLockdown, t.makeLogHandler("SECURITY.LOCKDOWN"))
	t.bus.Subscribe(kernel.EvSecAudit, t.makeLogHandler("SECURITY.AUDIT"))
	t.bus.Subscribe(kernel.EvAuthUnlock, t.makeLogHandler("AUTH.UNLOCK"))
	t.bus.Subscribe(kernel.EvAuthSetup, t.makeLogHandler("AUTH.SETUP"))
	t.bus.Subscribe(kernel.EvAuthLogout, t.makeLogHandler("AUTH.LOGOUT"))
	t.bus.Subscribe(kernel.EvAuthInitReady, t.makeLogHandler("AUTH.INIT_READY"))
	// Vault lifecycle events
	t.bus.Subscribe(kernel.EvStorageReady, t.makeLogHandler("STORAGE.READY"))
	t.bus.Subscribe(kernel.EvStorageVFSMount, t.makeLogHandler("STORAGE.VFS_MOUNT"))
}

// SetMVKResolver injects the function used to fetch the current Master Vault Key
// from locked memory (security.Module.RetrieveMVK). The translator uses this to
// encrypt/decrypt legacy entries that were written directly via wsSaveEntry.
// The returned slice must not be held past the current call frame — it points
// directly into locked memory managed by the security module.
func (t *Translator) SetMVKResolver(fn func() []byte) {
	t.mvkResolver = fn
}

// SetSessionKey sets the per-session ChaCha20-Poly1305 key used for SKE.
// Called once after a successful unlock. The key is shared with the frontend
// (sent base64-encoded in the unlock response) and is held only in RAM on
// both sides. It is zeroed on lock/logout.
func (t *Translator) SetSessionKey(key []byte) {
	t.sessionKey = key
	t.sessionKeySet = true
	log.Printf("[translator] Session key set (len=%d)", len(key))
}

// SetSessionKeyHandle sets the Rust enclave handle for the session key.
// When available, SKE operations will use the Rust enclave instead of Go's heap.
func (t *Translator) SetSessionKeyHandle(handle string) {
	t.sessionKeyHandle = handle
	log.Printf("[translator] Session key handle set: %s", handle)
}

// skeEncrypt encrypts plaintext with the session key using ChaCha20-Poly1305.
// Returns base64(nonce[12] + ciphertext+tag).
// Tries the Rust enclave first (handle-based encryption), falls back to Go.
func (t *Translator) skeEncrypt(plaintext []byte) (string, error) {
	if !t.sessionKeySet || len(t.sessionKey) != 32 {
		return "", fmt.Errorf("session key not available")
	}

	// If the Rust enclave has a handle, use it for encryption.
	if t.sessionKeyHandle != "" {
		ct, err := rustbridge.EncryptHandle(t.sessionKeyHandle, plaintext, nil)
		if err != nil {
			log.Printf("[translator:skeEncrypt] Rust enclave failed, falling back to Go: %v", err)
		} else {
			return base64.StdEncoding.EncodeToString(ct), nil
		}
	}

	// Go fallback: encrypt directly with the session key.
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("ske nonce: %w", err)
	}
	cipher, err := chacha20poly1305.New(t.sessionKey)
	if err != nil {
		return "", fmt.Errorf("ske cipher: %w", err)
	}
	ct := cipher.Seal(nil, nonce, plaintext, nil)
	// nonce(12) || ciphertext+tag
	blob := make([]byte, 0, len(nonce)+len(ct))
	blob = append(blob, nonce...)
	blob = append(blob, ct...)
	return base64.StdEncoding.EncodeToString(blob), nil
}

func generateID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("translator: CSPRNG failure: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// SetSyncFns injects the callbacks used for LAN Sync IPC (MsgSyncListPeers, MsgSyncTrigger).
// peersFn must return JSON-encoded sync state; triggerFn fires an immediate sync cycle.
// Both are optional; if nil the corresponding messages return "sync unavailable".
func (t *Translator) SetSyncFns(peersFn func() ([]byte, error), triggerFn func()) {
	t.syncPeersFn = peersFn
	t.syncTriggerFn = triggerFn
}

// SetAuditLog injects the security AuditLog used to serve MsgAuditList requests.
func (t *Translator) SetAuditLog(al security.AuditLog) {
	t.auditLog = al
}

// SetTokenValidator injects the function used to validate the one-time token
// sent by the UI during the three-way WebSocket handshake (MsgAuthTokenSubmit).
// The token is generated at daemon startup via GenerateSecureToken() and
// sent to Tauri as a cookie; the UI echoes it back to prove it originates
// from the legitimate Tauri shell, not an external browser tab.
func (t *Translator) SetTokenValidator(fn func(string) bool) {
	t.tokenValidator = fn
}

// makeLogHandler returns a handler that broadcasts security events as log messages.
func (t *Translator) makeLogHandler(eventName string) func(kernel.Event) error {
	return func(ev kernel.Event) error {
		if t.bridge == nil {
			return nil
		}
		logMsg := fmt.Sprintf("[%s] %s", eventName, string(ev.Payload))
		payload, _ := json.Marshal(map[string]string{"message": logMsg})
		t.bridge.Broadcast(ipc.MsgLogBroadcast, payload)
		return nil
	}
}

// HandleWS is the ws.MessageHandler for WebSocket connections.
func (t *Translator) HandleWS(msgType byte, payload []byte, conn *gorillaws.Conn) error {
	switch msgType {
	case ipc.MsgGetHeader:
		return t.wsGetHeader(conn)

	case ipc.MsgGetCiphertext:
		return t.wsGetCiphertext(conn)

	case ipc.MsgUpdateHeader:
		return t.wsUpdateHeader(payload)

	case ipc.MsgUpdateCiphertext:
		return t.wsUpdateCiphertext(payload)

	case ipc.MsgCheckVaultStatus:
		return t.wsCheckVaultStatus(conn)

	case ipc.MsgInitializeVault:
		return t.wsInitializeVault(conn, payload)

	case ipc.MsgUnlockVault:
		return t.wsUnlockVault(conn, payload)

	case ipc.MsgSaveEntry:
		return t.wsSaveEntry(conn, payload)

	case ipc.MsgListEntries:
		return t.wsListEntries(conn)

	case ipc.MsgGetEntry:
		return t.wsGetEntry(conn, payload)

	case ipc.MsgDeleteEntry:
		return t.wsDeleteEntry(conn, payload)

	case ipc.MsgDecryptEntry:
		return t.wsDecryptEntry(conn, payload)

	case ipc.MsgResetVault:
		return t.wsResetVault(conn, payload)

	case ipc.MsgChangePasswordRecovery:
		return t.wsChangePasswordWithRecovery(conn, payload)

	case ipc.MsgGenerateMatrix:
		return t.wsGenerateMatrix(conn, payload)

	case ipc.MsgTriggerWipe:
		log.Printf("[Translator] TriggerWipe received")
		return ws.WriteMessage(conn, ipc.MsgAck, nil)

	case ipc.MsgPanicWipe:
		log.Printf("[Translator] PanicWipe received — dispatching SECURITY.PANIC")
		ev := kernel.NewEvent("api", kernel.EvSecPanic, []byte(`{"reason":"panic_wipe","source":"frontend"}`))
		_ = t.bus.Dispatch(ev)
		return ws.WriteMessage(conn, ipc.MsgAck, nil)

	case ipc.MsgZeroizeConfirm:
		return ws.WriteMessage(conn, ipc.MsgAck, nil)

	case ipc.MsgEntryCreate:
		if t.entryHandler == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
		}
		return t.entryHandler.HandleCreate(conn, payload)

	case ipc.MsgEntryUpdate:
		if t.entryHandler == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
		}
		return t.entryHandler.HandleUpdate(conn, payload)

	case ipc.MsgEntryDelete:
		if t.entryHandler == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
		}
		return t.entryHandler.HandleDelete(conn, payload)

	case ipc.MsgFileIngestBegin:
		if t.entryHandler == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
		}
		return t.entryHandler.HandleIngestBegin(conn, payload)

	case ipc.MsgFileChunk:
		if t.entryHandler == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
		}
		return t.entryHandler.HandleChunk(conn, payload)

	case ipc.MsgFileIngestEnd:
		if t.entryHandler == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
		}
		return t.entryHandler.HandleIngestEnd(conn)

	case ipc.MsgGetRecoveryPhrase:
		return t.wsGetRecoveryPhrase(conn, payload)

	case ipc.MsgPanicWipeRequest:
		return t.wsPanicWipe(conn, payload)

	case ipc.MsgWorkspaceList:
		return t.wsListWorkspaces(conn)

	case ipc.MsgWorkspaceCreate:
		return t.wsCreateWorkspace(conn, payload)

	case ipc.MsgWorkspaceSwitch:
		return t.wsSwitchWorkspace(conn, payload)

	case ipc.MsgWorkspaceDelete:
		return t.wsDeleteWorkspace(conn, payload)

	case ipc.MsgWorkspaceRename:
		return t.wsRenameWorkspace(conn, payload)

	case ipc.MsgFileDownloadRequest:
		return t.wsFileDownload(conn, payload)

	// FileVault folder operations
	case ipc.MsgFolderCreate:
		if t.entryHandler != nil {
			return t.entryHandler.HandleFolderCreate(conn, payload)
		}
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
	case ipc.MsgFolderList:
		if t.entryHandler != nil {
			return t.entryHandler.HandleFolderList(conn, payload, t.skeEncrypt)
		}
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
	case ipc.MsgFolderRename:
		if t.entryHandler != nil {
			return t.entryHandler.HandleFolderRename(conn, payload)
		}
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
	case ipc.MsgFolderDelete:
		if t.entryHandler != nil {
			return t.entryHandler.HandleFolderDelete(conn, payload)
		}
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
	case ipc.MsgFileMoveToFolder:
		if t.entryHandler != nil {
			return t.entryHandler.HandleFileMoveToFolder(conn, payload)
		}
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))

	case ipc.MsgPanicButton:
		return t.wsPanicButton(conn, payload)

	case ipc.MsgAuthTokenSubmit:
		return t.wsAuthTokenSubmit(conn, payload)

	case ipc.MsgEntryQuery:
		return t.wsEntryQuery(conn, payload)

	case ipc.MsgSSHKeyGen:
		return t.wsSSHKeyGen(conn, payload)

	case ipc.MsgReconnect:
		return t.wsReconnect(conn, payload)

	case ipc.MsgGQLQuery:
		return t.wsGQLQuery(conn, payload)

	case ipc.MsgSystemHeartbeat:
		// Client acknowledged heartbeat — no action needed.
		return nil

	// ── LAN Sync IPC ─────────────────────────────────────────────────────────
	case ipc.MsgSyncListPeers:
		if t.syncPeersFn == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte(`{"error":"sync unavailable"}`))
		}
		data, err := t.syncPeersFn()
		if err != nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
		}
		enc, err := t.skeEncrypt(data)
		if err != nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
		}
		return ws.WriteMessage(conn, ipc.MsgSyncResult, []byte(enc))

	case ipc.MsgSyncTrigger:
		if t.syncTriggerFn == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte(`{"error":"sync unavailable"}`))
		}
		t.syncTriggerFn()
		return ws.WriteMessage(conn, ipc.MsgSyncResult, []byte(`{"ok":true}`))

	// ── Audit Log IPC ─────────────────────────────────────────────────────────
	case ipc.MsgAuditList:
		if t.auditLog == nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte(`{"error":"audit unavailable"}`))
		}
		n := 50
		if len(payload) >= 2 {
			n = int(binary.BigEndian.Uint16(payload))
		}
		if n <= 0 || n > 500 {
			n = 50
		}
		events := t.auditLog.Recent(n)
		data, _ := json.Marshal(events)
		enc, err := t.skeEncrypt(data)
		if err != nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
		}
		return ws.WriteMessage(conn, ipc.MsgAuditResult, []byte(enc))

	case ipc.MsgAuthLogout:
		if t.sessionCtx != nil {
			t.sessionCtx.Lock()
			// Zero the session key backing array before releasing the reference.
			// Setting to nil alone does not zero the underlying memory; the GC
			// may not collect it immediately, leaving the key accessible in a dump.
			if len(t.sessionKey) > 0 {
				for i := range t.sessionKey {
					t.sessionKey[i] = 0
				}
			}
			t.sessionKey = nil
			t.sessionKeySet = false
			t.sessionKeyHandle = ""
		}
		return ws.WriteMessage(conn, ipc.MsgAuthLogoutAck, nil)

	// ── TOTP / 2FA ───────────────────────────────────────────────────────────
	case ipc.MsgTOTPGenerate:
		return t.wsDispatch(conn, kernel.EvTOTPGenerate, payload, ipc.MsgTOTPResult, "TOTP.GENERATE")

	// ── Password Health Analysis ──────────────────────────────────────────────
	case ipc.MsgHealthAnalyze:
		return t.wsDispatch(conn, kernel.EvHealthAnalyze, payload, ipc.MsgHealthResult, "HEALTH.ANALYZE")

	// ── CSV Import ────────────────────────────────────────────────────────────
	case ipc.MsgImportCSV:
		return t.wsDispatch(conn, kernel.EvImportCSV, payload, ipc.MsgImportResult, "IMPORT.CSV")

	// ── Entry Version History ─────────────────────────────────────────────────
	case ipc.MsgEntryHistory:
		return t.wsDispatch(conn, kernel.EvEntryHistory, payload, ipc.MsgEntryHistoryResult, "ENTRY.HISTORY")

	case ipc.MsgEntryRestore:
		return t.wsDispatch(conn, kernel.EvEntryRestore, payload, ipc.MsgEntryHistoryResult, "ENTRY.RESTORE")

	// ── Shamir Secret Sharing ─────────────────────────────────────────────────
	case ipc.MsgShamirSplit:
		return t.wsDispatch(conn, kernel.EvShamirSplit, payload, ipc.MsgShamirResult, "SHAMIR.SPLIT")

	case ipc.MsgShamirCombine:
		return t.wsDispatch(conn, kernel.EvShamirCombine, payload, ipc.MsgShamirResult, "SHAMIR.COMBINE")

	// ── Secure Share ──────────────────────────────────────────────────────────
	case ipc.MsgShareCreate:
		return t.wsDispatch(conn, kernel.EvShareCreate, payload, ipc.MsgShareResult, "SHARE.CREATE")

	case ipc.MsgShareRedeem:
		return t.wsDispatch(conn, kernel.EvShareRedeem, payload, ipc.MsgShareResult, "SHARE.REDEEM")

	case ipc.MsgShareRevoke:
		return t.wsDispatch(conn, kernel.EvShareRevoke, payload, ipc.MsgShareResult, "SHARE.REVOKE")

	// ── Air-Gap Backup ────────────────────────────────────────────────────────
	case ipc.MsgBackupExport:
		return t.wsDispatch(conn, kernel.EvBackupExport, payload, ipc.MsgBackupResult, "BACKUP.EXPORT")

	case ipc.MsgBackupPeek:
		return t.wsDispatch(conn, kernel.EvBackupPeek, payload, ipc.MsgBackupResult, "BACKUP.PEEK")

	case ipc.MsgBackupAuthorize:
		return t.wsDispatch(conn, kernel.EvBackupAuthorize, payload, ipc.MsgBackupResult, "BACKUP.AUTHORIZE")

	case ipc.MsgBackupChecksum:
		return t.wsDispatch(conn, kernel.EvBackupChecksum, payload, ipc.MsgBackupResult, "BACKUP.CHECKSUM")

	default:
		return fmt.Errorf("unknown message type 0x%02x", msgType)
	}
}

// emitSystemError broadcasts a SYSTEM.ERROR event to all connected clients.
func (t *Translator) emitSystemError(msg string) {
	if t.bridge == nil {
		return
	}
	payload, _ := json.Marshal(map[string]string{"error": msg, "source": "kernel"})
	t.bridge.Broadcast(ipc.MsgError, payload)
}

// wsError writes a structured error response to the WebSocket client.
// If err is a *GrimlockError, the error_code is included in the JSON payload.
// Falls back to plain text for non-typed errors.
func wsError(conn *gorillaws.Conn, err error) error {
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		payload, _ := json.Marshal(map[string]interface{}{
			"error":      ge.Message,
			"error_code": ge.Code,
		})
		return ws.WriteMessage(conn, ipc.MsgError, payload)
	}
	return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
}

// wsErrorMsg writes a plain string error (for cases where we have no typed error).
func wsErrorMsg(conn *gorillaws.Conn, msg string) error {
	return ws.WriteMessage(conn, ipc.MsgError, []byte(msg))
}

// --- Header / Ciphertext passthrough (unchanged from grimdb-go) ---

func (t *Translator) wsGetHeader(conn *gorillaws.Conn) error {
	h := t.db.GetHeader()
	buf := headerToBytes(h)
	return ws.WriteMessage(conn, ipc.MsgHeader, buf)
}

func (t *Translator) wsGetCiphertext(conn *gorillaws.Conn) error {
	ct, err := t.db.GetCiphertext()
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}
	return ws.WriteMessage(conn, ipc.MsgCiphertext, ct)
}

func (t *Translator) wsUpdateHeader(payload []byte) error {
	if len(payload) != grimdb.HeaderSize {
		return fmt.Errorf("header payload: got %d want %d", len(payload), grimdb.HeaderSize)
	}
	h := bytesToHeader(payload)
	return t.db.UpdateHeader(h)
}

func (t *Translator) wsUpdateCiphertext(payload []byte) error {
	return t.db.UpdateCiphertext(payload)
}

// --- Vault status ---

func (t *Translator) wsCheckVaultStatus(conn *gorillaws.Conn) error {
	initialized, isV5, _ := grimdb.CheckVaultStatus(t.appDir)
	status, _ := json.Marshal(map[string]bool{"initialized": initialized, "isV5": isV5})
	return ws.WriteMessage(conn, ipc.MsgAck, status)
}

// --- Vault init/unlock (dispatch AUTH events, wait for AUTH.RESULT) ---

func (t *Translator) wsInitializeVault(conn *gorillaws.Conn, payload []byte) error {
	password := string(payload)

	phrase, err := grimdb.InitializeVault(password, t.appDir)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	ev := kernel.NewEvent("api", kernel.EvAuthSetup, []byte(`{"initialized":true}`))
	_ = t.bus.Dispatch(ev)

	if err := ws.WriteMessage(conn, ipc.MsgRecoveryPhrase, []byte(phrase)); err != nil {
		return err
	}

	// Auto-unlock immediately after init: vault was just created with this password,
	// so unlock must succeed. Without this, the STORAGE gate stays closed and the
	// first STORAGE.WRITE (e.g. initial entry save) would be dropped.
	log.Printf("[translator] auto-unlocking after vault init")
	evPayload, _ := json.Marshal(map[string]string{"password": password, "app_dir": t.appDir})
	// Zero the JSON payload after the request completes — it contains the password.
	defer func() {
		for i := range evPayload {
			evPayload[i] = 0
		}
	}()
	unlockEv := kernel.NewEvent("api", kernel.EvAuthUnlock, evPayload)
	log.Printf("[translator:unlock] RequestID=%s PayloadLen=%d", unlockEv.ID, len(evPayload))
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	result, err := t.bus.Request(ctx, unlockEv)
	if err != nil {
		log.Printf("[translator] auto-unlock Request error: %v", err)
		return nil
	}
	log.Printf("[translator:unlock] REPLY: ID=%s Type=%s ReplyTo=%s Payload=%s",
		result.ID, result.Type, result.ReplyTo, string(result.Payload))
	var res struct {
		Success    bool   `json:"success"`
		Reason     string `json:"reason,omitempty"`
		SessionKey string `json:"session_key,omitempty"`
	}
	_ = json.Unmarshal(result.Payload, &res)
	if !res.Success {
		log.Printf("[translator] auto-unlock failed: %s", res.Reason)
	} else {
		log.Printf("[translator] auto-unlock succeeded — STORAGE gate open")
		// The onSessionKey callback should have already set the key on the translator,
		// but also send a MsgUnlockResult with the session key so the frontend
		// receives it after initialization.
		if res.SessionKey != "" {
			unlockResp, _ := json.Marshal(map[string]interface{}{
				"success":     true,
				"session_key": res.SessionKey,
			})
			return ws.WriteMessage(conn, ipc.MsgUnlockResult, unlockResp)
		}
	}
	return nil
}

func (t *Translator) wsUnlockVault(conn *gorillaws.Conn, payload []byte) error {
	// Copy password to a local slice so we can zero it after use.
	// Go strings are immutable and cannot be zeroed, but []byte can.
	passwordBytes := make([]byte, len(payload))
	copy(passwordBytes, payload)
	defer func() {
		for i := range passwordBytes {
			passwordBytes[i] = 0
		}
	}()

	evPayload, _ := json.Marshal(map[string]string{"password": string(passwordBytes), "app_dir": t.appDir})
	// Zero the JSON payload after use — it contains the password in plaintext.
	defer func() {
		for i := range evPayload {
			evPayload[i] = 0
		}
	}()

	ev := kernel.NewEvent("api", kernel.EvAuthUnlock, evPayload)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return wsError(conn, gerrors.NewBusTimeoutError("AUTH.UNLOCK"))
	}

	var res struct {
		Success    bool   `json:"success"`
		Reason     string `json:"reason,omitempty"`
		SessionKey string `json:"session_key,omitempty"`
	}
	_ = json.Unmarshal(result.Payload, &res)

	if !res.Success {
		reason := res.Reason
		if reason == "" {
			reason = "invalid password"
		}
		return wsError(conn, gerrors.NewAuthInvalidError("vault_unlock", nil).
			WithDetails("reason", reason))
	}

	// Store the session key on the translator if it wasn't already set
	// by the onSessionKey callback (belt-and-suspenders).
	if res.SessionKey != "" && !t.sessionKeySet {
		if keyBytes, err := base64.StdEncoding.DecodeString(res.SessionKey); err == nil && len(keyBytes) == 32 {
			t.SetSessionKey(keyBytes)
		}
	}

	// Send JSON response including the session key so the frontend can
	// decrypt SKE-encrypted data locally.
	resp, _ := json.Marshal(map[string]interface{}{
		"success":     true,
		"session_key": res.SessionKey,
	})
	return ws.WriteMessage(conn, ipc.MsgUnlockResult, resp)
}

// --- Entry operations (dispatch STORAGE events) ---

func (t *Translator) wsSaveEntry(conn *gorillaws.Conn, payload []byte) error {
	// Frontend sends {type, category, data} — transform into encrypted Block.
	var raw struct {
		Type     string          `json:"type"`
		Category string          `json:"category"`
		Data     json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return wsError(conn, gerrors.NewProtocolError("save_entry_unmarshal", err))
	}

	entryID := generateID()

	// Resolve category: explicit category > type-based fallback
	cat := raw.Category
	if cat == "" {
		cat = strings.ToUpper(raw.Type)
	}

	entry := map[string]interface{}{
		"id":       entryID,
		"type":     raw.Type,
		"category": cat,
		"data":     raw.Data,
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("marshal entry failed"))
	}

	// Encrypt the entry JSON with the master vault key.
	mk := t.mvkResolver()
	if mk == nil {
		return wsError(conn, gerrors.NewVaultLockedError())
	}
	nonce, err := t.crypto.NewNonce()
	if err != nil {
		return wsError(conn, gerrors.NewCryptoEncryptionError("new_nonce", err))
	}
	ct, err := t.crypto.Encrypt(mk, nonce[:], entryJSON, nil)
	if err != nil {
		return wsError(conn, gerrors.NewCryptoEncryptionError("entry_encrypt", err))
	}
	// Prepend nonce to ciphertext so the blob is self-contained.
	blob := make([]byte, 12+len(ct))
	copy(blob[:12], nonce[:])
	copy(blob[12:], ct)

	blockPayload, _ := json.Marshal(map[string]interface{}{
		"block": storage.Block{
			ID:   entryID,
			Data: blob,
		},
	})

	ev := kernel.NewEvent("api", kernel.EvStorageWrite, blockPayload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return wsError(conn, gerrors.NewBusTimeoutError("STORAGE.WRITE"))
	}
	var res struct {
		Error     string `json:"error"`
		ErrorCode int    `json:"error_code,omitempty"`
	}
	_ = json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		if res.ErrorCode != 0 {
			ge := &gerrors.GrimlockError{Code: res.ErrorCode, Message: res.Error}
			return wsError(conn, ge)
		}
		return wsErrorMsg(conn, res.Error)
	}

	log.Printf("[translator:SAVE] entryID=%s type=%s blobLen=%d", entryID, raw.Type, len(blob))
	ack, _ := json.Marshal(map[string]string{"id": entryID, "status": "saved"})
	return ws.WriteMessage(conn, ipc.MsgAck, ack)
}

func (t *Translator) wsListEntries(conn *gorillaws.Conn) error {
	ev := kernel.NewEvent("api", kernel.EvStorageList, nil)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return wsError(conn, gerrors.NewBusTimeoutError("STORAGE.LIST"))
	}

	var stored struct {
		Metas []storage.BlockMeta `json:"metas,omitempty"`
		Error string              `json:"error,omitempty"`
	}
	if err := json.Unmarshal(result.Payload, &stored); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid list response"))
	}
	if stored.Error != "" {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(stored.Error))
	}

	type entrySummary struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Category  string `json:"category,omitempty"`
		Title     string `json:"title,omitempty"`
		CreatedAt int64  `json:"created_at"`
		UpdatedAt int64  `json:"updated_at"`
	}
	summaries := make([]entrySummary, 0, len(stored.Metas))
	mk := t.mvkResolver()
	if mk == nil {
		return wsError(conn, gerrors.NewVaultLockedError())
	}

	for _, meta := range stored.Metas {
		// Skip raw chunk blocks — they contain encrypted binary, not entry data.
		// File vault manifests use format "blob-UUID-manifest", chunks use "blob-UUID-chunk-N".
		// We skip chunks here; manifests are parsed in Path 3 below.
		if strings.Contains(meta.ID, "-chunk-") {
			continue
		}

		sum := entrySummary{
			ID:        meta.ID,
			Category:  string(meta.Category), // always available from the block index
			CreatedAt: meta.CreatedAt,
			UpdatedAt: meta.UpdatedAt,
		}
		if meta.ID != "" {
			reqPayload, _ := json.Marshal(map[string]string{"id": meta.ID})
			readEv := kernel.NewEvent("api", kernel.EvStorageRead, reqPayload)
			readCtx, cancelRead := context.WithTimeout(context.Background(), requestTimeout)
			readResult, err := t.bus.Request(readCtx, readEv)
			cancelRead()
			if err == nil {
				var rd struct {
					Block *storage.Block `json:"block,omitempty"`
				}
				json.Unmarshal(readResult.Payload, &rd)
				if rd.Block != nil {
					data := rd.Block.Data
					decoded := false

					// ── Path 1: legacy encrypted format (nonce[12] + MVK ciphertext) ──
					if mk != nil && len(data) >= 12 {
						pt, decErr := t.crypto.Decrypt(mk, data[:12], data[12:], nil)
						if decErr == nil {
							var entry struct {
								Type string          `json:"type"`
								Data json.RawMessage `json:"data"`
							}
							if json.Unmarshal(pt, &entry) == nil {
								sum.Type = entry.Type
								var entryData struct {
									Title string `json:"title"`
								}
								if json.Unmarshal(entry.Data, &entryData) == nil {
									sum.Title = entryData.Title
								}
								decoded = true
							}
						}
					}

					// ── Path 2: plain VaultEntry JSON (new-path: SSH gen, entry module) ──
					if !decoded {
						var ve storage.VaultEntry
						if json.Unmarshal(data, &ve) == nil && ve.ID != "" {
							sum.Type = ve.Type
							if sum.Type == "" {
								sum.Type = string(ve.Category)
							}
							sum.Category = string(ve.Category)
							sum.Title = ve.Title
						}
					}

					// ── Path 3: File vault manifest (BlobManifest JSON) ──
					if !decoded && strings.Contains(meta.ID, "-manifest") {
						var manifest storage.BlobManifest
						if json.Unmarshal(data, &manifest) == nil && manifest.ID != "" {
							sum.Type = "file_vault"
							sum.Category = string(storage.CategoryFileVault)
							sum.Title = manifest.FileName
							decoded = true
						}
					}
				}
			}
		}
		summaries = append(summaries, sum)
	}

	plainJSON, _ := json.Marshal(summaries)
	encData, err := t.skeEncrypt(plainJSON)
	if err != nil {
		log.Printf("[translator:LIST] SKE encrypt failed: %v", err)
		return ws.WriteMessage(conn, ipc.MsgError, []byte("session encryption unavailable"))
	}

	resp, _ := json.Marshal(map[string]string{"encrypted": encData})
	log.Printf("[translator:LIST] %d entries returned (SKE)", len(summaries))
	return ws.WriteMessage(conn, ipc.MsgEntriesResult, resp)
}

func (t *Translator) wsGetEntry(conn *gorillaws.Conn, payload []byte) error {
	reqPayload, _ := json.Marshal(map[string]string{"id": string(payload)})
	ev := kernel.NewEvent("api", kernel.EvStorageRead, reqPayload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return wsError(conn, gerrors.NewBusTimeoutError("STORAGE.READ"))
	}

	var stored struct {
		Block     *storage.Block `json:"block,omitempty"`
		Error     string         `json:"error,omitempty"`
		ErrorCode int            `json:"error_code,omitempty"`
	}
	if err := json.Unmarshal(result.Payload, &stored); err != nil {
		return wsError(conn, gerrors.NewProtocolError("get_entry_unmarshal", err))
	}
	if stored.Error != "" || stored.Block == nil {
		if stored.ErrorCode == gerrors.ErrCodeStorageNotFound {
			return wsError(conn, gerrors.NewStorageNotFoundError(string(payload)))
		}
		msg := "entry not found"
		if stored.Error != "" {
			msg = stored.Error
		}
		return wsErrorMsg(conn, msg)
	}

	mk := t.mvkResolver()
	if mk == nil {
		return wsError(conn, gerrors.NewVaultLockedError())
	}

	blob := stored.Block.Data
	if len(blob) < 12 {
		return wsError(conn, gerrors.NewStorageCorruptionError("entry_blob_too_short", string(payload), nil))
	}

	var metadata map[string]interface{}

	// ── Path 1: legacy encrypted format (nonce[12] + MVK ciphertext) ──
	if len(blob) >= 12 {
		pt, decErr := t.crypto.Decrypt(mk, blob[:12], blob[12:], nil)
		if decErr == nil {
			var fullEntry struct {
				ID   string          `json:"id"`
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(pt, &fullEntry) == nil {
				metadata = map[string]interface{}{
					"id":         stored.Block.ID,
					"type":       fullEntry.Type,
					"created_at": stored.Block.CreatedAt,
					"updated_at": stored.Block.UpdatedAt,
				}
				var entryData struct {
					Title string `json:"title"`
				}
				if json.Unmarshal(fullEntry.Data, &entryData) == nil {
					metadata["title"] = entryData.Title
				}
			}
		}
	}

	// ── Path 2: plain VaultEntry JSON (SSH gen, entry module) ──
	if metadata == nil {
		var se struct {
			storage.VaultEntry
			PrivateKeyPEM string `json:"privateKeyPEM"`
		}
		if json.Unmarshal(blob, &se) == nil && se.ID != "" {
			metadata = map[string]interface{}{
				"id":         se.ID,
				"type":       se.Type,
				"category":   string(se.Category),
				"title":      se.Title,
				"created_at": stored.Block.CreatedAt,
				"updated_at": stored.Block.UpdatedAt,
			}
			if pk, ok := se.Fields["publicKey"]; ok && pk != "" {
				metadata["publicKey"] = pk
			}
			if fp, ok := se.Fields["fingerprint"]; ok && fp != "" {
				metadata["fingerprint"] = fp
			}
		}
	}

	// ── Path 3: File vault manifest (BlobManifest JSON) ──
	if metadata == nil {
		var manifest storage.BlobManifest
		if json.Unmarshal(blob, &manifest) == nil && manifest.ID != "" {
			manifestBlockID := manifest.ManifestBlockID
			if manifestBlockID == "" {
				manifestBlockID = stored.Block.ID
			}
			metadata = map[string]interface{}{
				"id":                stored.Block.ID,
				"type":              "file_vault",
				"category":          string(storage.CategoryFileVault),
				"title":             manifest.FileName,
				"file_name":         manifest.FileName,
				"mime_type":         manifest.MIMEType,
				"total_size":        manifest.TotalSize,
				"manifest_block_id": manifestBlockID,
				"created_at":        stored.Block.CreatedAt,
				"updated_at":        stored.Block.UpdatedAt,
			}
		}
	}

	if metadata == nil {
		log.Printf("[translator:GET] decode FAIL entryID=%s", string(payload))
		return ws.WriteMessage(conn, ipc.MsgError, []byte("decryption failed"))
	}

	metaJSON, _ := json.Marshal(metadata)
	encData, err := t.skeEncrypt(metaJSON)
	if err != nil {
		log.Printf("[translator:GET] SKE encrypt failed: %v", err)
		return ws.WriteMessage(conn, ipc.MsgError, []byte("session encryption unavailable"))
	}

	resp, _ := json.Marshal(map[string]string{"encrypted": encData})
	log.Printf("[translator:GET] entryID=%s metadata sent (SKE)", string(payload))
	return ws.WriteMessage(conn, ipc.MsgEntryData, resp)
}

// wsDecryptEntry decrypts the full entry data (including sensitive fields like
// passwords and SSH keys) and sends it SKE-encrypted to the frontend.
// This is the ONLY path that reveals sensitive data, and it requires an
// explicit user action (clicking "Reveal" in the UI).
func (t *Translator) wsDecryptEntry(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid decrypt request"))
	}

	reqPayload, _ := json.Marshal(map[string]string{"id": req.ID})
	ev := kernel.NewEvent("api", kernel.EvStorageRead, reqPayload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return wsError(conn, gerrors.NewBusTimeoutError("STORAGE.READ"))
	}

	var stored struct {
		Block *storage.Block `json:"block,omitempty"`
		Error string         `json:"error,omitempty"`
	}
	if err := json.Unmarshal(result.Payload, &stored); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid read response"))
	}
	if stored.Error != "" || stored.Block == nil {
		msg := "entry not found"
		if stored.Error != "" {
			msg = stored.Error
		}
		return ws.WriteMessage(conn, ipc.MsgError, []byte(msg))
	}

	mk := t.mvkResolver()
	if mk == nil {
		return wsError(conn, gerrors.NewVaultLockedError())
	}

	blob := stored.Block.Data
	if len(blob) < 12 {
		return wsError(conn, gerrors.NewStorageCorruptionError("decrypt_blob_too_short", req.ID, nil))
	}

	var decryptedResult map[string]interface{}

	// ── Path 1: legacy encrypted format (nonce[12] + MVK ciphertext) ──
	if len(blob) >= 12 {
		pt, decErr := t.crypto.Decrypt(mk, blob[:12], blob[12:], nil)
		if decErr == nil {
			var fullEntry struct {
				ID   string          `json:"id"`
				Type string          `json:"type"`
				Data json.RawMessage `json:"data"`
			}
			if json.Unmarshal(pt, &fullEntry) == nil {
				decryptedResult = map[string]interface{}{
					"id":         stored.Block.ID,
					"type":       fullEntry.Type,
					"data":       fullEntry.Data,
					"created_at": stored.Block.CreatedAt,
					"updated_at": stored.Block.UpdatedAt,
				}
			}
		}
	}

	// ── Path 2: plain VaultEntry JSON (SSH gen, entry module) ──
	// storedEntry embeds VaultEntry + optional PrivateKeyPEM for SSH keys.
	if decryptedResult == nil {
		var se struct {
			storage.VaultEntry
			PrivateKeyPEM string `json:"privateKeyPEM"`
		}
		if jsonErr := json.Unmarshal(blob, &se); jsonErr != nil || se.ID == "" {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("decryption failed"))
		}
		// Merge fields + PrivateKeyPEM into a single data map.
		// Field keys must be camelCase to match what the frontend (DetailPanel) reads.
		entryData := make(map[string]interface{})
		for k, v := range se.Fields {
			entryData[k] = v
		}
		if se.PrivateKeyPEM != "" {
			entryData["privateKey"] = se.PrivateKeyPEM
		}
		entryData["title"] = se.Title
		dataJSON, _ := json.Marshal(entryData)
		decryptedResult = map[string]interface{}{
			"id":         se.ID,
			"type":       se.Type,
			"category":   string(se.Category),
			"data":       json.RawMessage(dataJSON),
			"created_at": se.CreatedAt,
			"updated_at": se.UpdatedAt,
		}
		log.Printf("[translator:DECRYPT] VaultEntry path entryID=%s category=%s", se.ID, se.Category)
	}

	// ── Path 3: File vault manifest (BlobManifest JSON) ──
	if decryptedResult == nil {
		var manifest storage.BlobManifest
		if json.Unmarshal(blob, &manifest) == nil && manifest.ID != "" {
			// Use manifest_block_id from the manifest if available; fall back to block ID.
			manifestBlockID := manifest.ManifestBlockID
			if manifestBlockID == "" {
				manifestBlockID = stored.Block.ID
			}
			manifestData := map[string]interface{}{
				"title":             manifest.FileName,
				"file_name":         manifest.FileName,
				"mime_type":         manifest.MIMEType,
				"total_size":        manifest.TotalSize,
				"sha256":            fmt.Sprintf("%x", manifest.SHA256),
				"manifest_block_id": manifestBlockID,
			}
			dataJSON, _ := json.Marshal(manifestData)
			decryptedResult = map[string]interface{}{
				"id":         stored.Block.ID,
				"type":       "file_vault",
				"category":   string(storage.CategoryFileVault),
				"data":       json.RawMessage(dataJSON),
				"created_at": stored.Block.CreatedAt,
				"updated_at": stored.Block.UpdatedAt,
			}
			log.Printf("[translator:DECRYPT] BlobManifest path entryID=%s file=%s", manifest.ID, manifest.FileName)
		}
	}

	if decryptedResult == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("decryption failed"))
	}

	resultJSON, _ := json.Marshal(decryptedResult)
	encData, err := t.skeEncrypt(resultJSON)
	if err != nil {
		log.Printf("[translator:DECRYPT] SKE encrypt failed: %v", err)
		return ws.WriteMessage(conn, ipc.MsgError, []byte("session key not available"))
	}

	resp, _ := json.Marshal(map[string]string{"encrypted": encData})
	log.Printf("[translator:DECRYPT] entryID=%s full data sent (SKE)", req.ID)
	return ws.WriteMessage(conn, ipc.MsgDecryptedData, resp)
}

func (t *Translator) wsDeleteEntry(conn *gorillaws.Conn, payload []byte) error {
	reqPayload, _ := json.Marshal(map[string]string{"id": string(payload)})
	ev := kernel.NewEvent("api", kernel.EvStorageDelete, reqPayload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return wsError(conn, gerrors.NewBusTimeoutError("STORAGE.DELETE"))
	}

	var res struct {
		Error     string `json:"error"`
		ErrorCode int    `json:"error_code,omitempty"`
	}
	_ = json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		if res.ErrorCode != 0 {
			return wsError(conn, &gerrors.GrimlockError{Code: res.ErrorCode, Message: res.Error})
		}
		return wsErrorMsg(conn, res.Error)
	}
	return ws.WriteMessage(conn, ipc.MsgAck, nil)
}

func (t *Translator) wsResetVault(conn *gorillaws.Conn, payload []byte) error {
	if err := grimdb.ResetVault(string(payload), t.appDir); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid recovery phrase"))
	}

	ev := kernel.NewEvent("api", kernel.EvSecLockdown, []byte(`{"reason":"vault_reset"}`))
	_ = t.bus.Dispatch(ev)

	return ws.WriteMessage(conn, ipc.MsgAck, nil)
}

func (t *Translator) wsChangePasswordWithRecovery(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		RecoveryPhrase string `json:"recovery_phrase"`
		NewPassword    string `json:"new_password"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}
	if req.RecoveryPhrase == "" || req.NewPassword == "" {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("recovery_phrase and new_password required"))
	}

	newPhrase, err := grimdb.ChangePasswordWithRecovery(req.RecoveryPhrase, req.NewPassword, t.appDir)
	if err != nil {
		log.Printf("[translator] ChangePasswordWithRecovery failed: %v", err)
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	// Lock the vault — user must log in fresh with new password
	ev := kernel.NewEvent("api", kernel.EvSecLockdown, []byte(`{"reason":"password_changed"}`))
	_ = t.bus.Dispatch(ev)

	result, _ := json.Marshal(map[string]interface{}{
		"success":             true,
		"new_recovery_phrase": newPhrase,
	})
	return ws.WriteMessage(conn, ipc.MsgChangePasswordResult, result)
}

func (t *Translator) wsGenerateMatrix(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		LineCount   int    `json:"line_count"`
		EntropyPath string `json:"entropy_path"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}

	entropyPath := filepath.Join(t.appDir, "entropy.bin")
	if req.EntropyPath != "" {
		// Security: only allow paths within the app directory
		clean := filepath.Clean(req.EntropyPath)
		if filepath.IsAbs(clean) {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entropy path must be relative"))
		}
		if strings.Contains(clean, "..") {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("entropy path must not traverse directories"))
		}
		entropyPath = filepath.Join(t.appDir, clean)
	}

	// Generate entropy file with progress streaming.
	if err := t.crypto.GenerateEntropyFileWithProgress(entropyPath, func(pct float64, msg string) {
		progPayload, _ := json.Marshal(map[string]interface{}{
			"progress": pct,
			"stage":    "generating",
			"message":  msg,
		})
		_ = ws.WriteMessage(conn, ipc.MsgProgress, progPayload)
	}); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(fmt.Sprintf("entropy generation failed: %v", err)))
	}

	// Load the entropy file to compute coordinates and key.
	entropyData, err := os.ReadFile(entropyPath)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(fmt.Sprintf("read entropy: %v", err)))
	}

	// For now, return a placeholder result. In a real implementation, this would:
	// 1. Allow the user to set a password
	// 2. Derive Argon2id hash from password
	// 3. Compute coordinate offsets
	// 4. Compute MVK
	// 5. Return the coordinates to the user for manual note-taking

	result, _ := json.Marshal(map[string]interface{}{
		"key_hex":         fmt.Sprintf("%x", entropyData[:16]),
		"coordinates":     []int{1, 2, 3}, // placeholder
		"entropy_file":    entropyPath,
		"entropy_size":    len(entropyData),
		"bits_of_entropy": len(entropyData) * 8,
	})
	return ws.WriteMessage(conn, ipc.MsgGenerationResult, result)
}

// --- Recovery and Security Operations ---

func (t *Translator) wsGetRecoveryPhrase(conn *gorillaws.Conn, payload []byte) error {
	password := string(payload)

	phrase, err := grimdb.RetrieveRecoveryPhrase(password, t.appDir)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	// Recovery phrase must always be SKE-encrypted before transmission.
	if !t.sessionKeySet {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("session key not available — unlock vault first"))
	}

	encData, encErr := t.skeEncrypt([]byte(phrase))
	if encErr != nil {
		log.Printf("[translator:RECOVERY] SKE encrypt failed: %v", encErr)
		return ws.WriteMessage(conn, ipc.MsgError, []byte("encryption failed"))
	}

	resp, _ := json.Marshal(map[string]string{"encrypted": encData})
	return ws.WriteMessage(conn, ipc.MsgRecoveryPhraseData, resp)
}

func (t *Translator) wsPanicWipe(conn *gorillaws.Conn, payload []byte) error {
	if err := grimdb.WipeVault(t.appDir); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(fmt.Sprintf("wipe failed: %v", err)))
	}

	// Dispatch security event for audit log
	ev := kernel.NewEvent("api", kernel.EvSecPanic, []byte(`{"reason":"panic_wipe_initiated","source":"api"}`))
	_ = t.bus.Dispatch(ev)

	return ws.WriteMessage(conn, ipc.MsgAck, nil)
}

// --- Binary header codec (preserves existing wire format) ---

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

// --- Workspace Management ---

func (t *Translator) wsListWorkspaces(conn *gorillaws.Conn) error {
	if t.workspaceMgr == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("workspace manager unavailable"))
	}

	workspaces := t.workspaceMgr.List()
	payload, _ := json.Marshal(workspaces)
	return ws.WriteMessage(conn, ipc.MsgWorkspacesResult, payload)
}

func (t *Translator) wsCreateWorkspace(conn *gorillaws.Conn, payload []byte) error {
	if t.workspaceMgr == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("workspace manager unavailable"))
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}

	if req.Name == "" || len(req.Name) > 128 {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("workspace name must be 1-128 characters"))
	}

	workspace, err := t.workspaceMgr.Create(req.Name)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	respPayload, _ := json.Marshal(workspace)
	return ws.WriteMessage(conn, ipc.MsgWorkspacesResult, respPayload)
}

func (t *Translator) wsSwitchWorkspace(conn *gorillaws.Conn, payload []byte) error {
	if t.workspaceMgr == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("workspace manager unavailable"))
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}

	workspace, err := t.workspaceMgr.Switch(req.ID)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	// Dispatch a WORKSPACE.SWITCH event for audit logging
	ev := kernel.NewEvent("api", kernel.EvWorkspaceSwitch, []byte(fmt.Sprintf(`{"workspace_id":"%s"}`, workspace.ID)))
	_ = t.bus.Dispatch(ev)

	respPayload, _ := json.Marshal(workspace)
	return ws.WriteMessage(conn, ipc.MsgWorkspacesResult, respPayload)
}

func (t *Translator) wsDeleteWorkspace(conn *gorillaws.Conn, payload []byte) error {
	if t.workspaceMgr == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("workspace manager unavailable"))
	}

	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}

	if err := t.workspaceMgr.Delete(req.ID); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	return ws.WriteMessage(conn, ipc.MsgAck, nil)
}

// wsRenameWorkspace renames a workspace by its ID.
func (t *Translator) wsRenameWorkspace(conn *gorillaws.Conn, payload []byte) error {
	if t.workspaceMgr == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("workspace manager unavailable"))
	}

	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}

	if err := t.workspaceMgr.Rename(req.ID, req.Name); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	workspaces := t.workspaceMgr.List()
	respPayload, _ := json.Marshal(workspaces)
	return ws.WriteMessage(conn, ipc.MsgWorkspacesResult, respPayload)
}

// wsFileDownload handles MsgFileDownloadRequest: decrypts and streams a file from the vault.
// The manifest_block_id must reference a BlobManifest block (CategoryFileVault).
// Chunks are streamed as MsgFileChunkData binary frames, followed by MsgFileDownloadEnd.
func (t *Translator) wsFileDownload(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		ManifestBlockID string `json:"manifest_block_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return wsError(conn, gerrors.NewProtocolError("file_download_unmarshal", err))
	}
	if req.ManifestBlockID == "" {
		return wsError(conn, gerrors.NewProtocolError("file_download_missing_id", nil))
	}

	if t.entryHandler == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry handler unavailable"))
	}

	// Retrieve MVK from locked memory.
	if t.mvkResolver == nil {
		return wsError(conn, gerrors.NewSecurityMVKMissingError("file_download"))
	}
	mvk := t.mvkResolver()
	if len(mvk) == 0 {
		return wsError(conn, gerrors.NewSecurityMVKMissingError("file_download"))
	}

	// Normalise the manifest block ID: if the frontend sent a bare UUID (legacy entries
	// before ManifestBlockID was added to BlobManifest), reconstruct the full block key.
	manifestBlockID := req.ManifestBlockID
	if !strings.HasPrefix(manifestBlockID, "blob-") {
		manifestBlockID = fmt.Sprintf("blob-%s-manifest", manifestBlockID)
		log.Printf("[translator:DOWNLOAD] Normalised bare UUID → %s", manifestBlockID)
	}

	// Delegate to the entry handler which has access to IngestEngine and bridge.
	return t.entryHandler.HandleFileDownload(conn, manifestBlockID, mvk, t.bridge)
}

// wsPanicButton handles MsgPanicButton: triggers hard lockdown (Admin-only).
// Requires passphrase confirmation before executing the lockdown.
// The passphrase must be non-empty and may be validated against a stored admin
// credential in future releases.
func (t *Translator) wsPanicButton(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return wsError(conn, gerrors.NewProtocolError("panic_button_unmarshal", err))
	}

	// Require a non-empty passphrase. In a future multi-user tier this should
	// be validated against an admin credential store.
	if req.Passphrase == "" {
		return wsError(conn, gerrors.NewAuthInvalidError("panic_button", nil).
			WithDetails("reason", "passphrase required for panic button"))
	}

	// Log the panic button activation attempt (without the passphrase).
	log.Printf("[PanicButton] PANIC BUTTON activated — triggering hard lockdown")

	// Dispatch SECURITY.PANIC to trigger the hard lockdown handler.
	panicPayload, _ := json.Marshal(map[string]string{
		"reason":   "panic_button_activated",
		"operator": "admin",
	})
	_ = t.bus.Dispatch(kernel.NewEvent("api", kernel.EvSecPanic, panicPayload))

	// Send acknowledgment before the connection is terminated.
	ackPayload, _ := json.Marshal(map[string]bool{"lockdown_initiated": true})
	return ws.WriteMessage(conn, ipc.MsgAck, ackPayload)
}

// wsGQLQuery handles MsgGQLQuery: binary GQL frames that go through the
// two-stage validator (syntactic + semantic/ACL) before reaching the dispatcher.
// This is the Phase 4 injection-immune query path.
//
// Frame format: GQL binary frame (8-byte header + binary payload).
// The payload is deserialized, validated against schema and ACL, then dispatched
// to the appropriate storage operation. Results are returned as MsgGQLResult.
func (t *Translator) wsGQLQuery(conn *gorillaws.Conn, payload []byte) error {
	if t.gqlDispatcher == nil {
		return ws.WriteMessage(conn, ipc.MsgGQLResult,
			gql.NewErrorFrame(-100, "GQL dispatcher unavailable").Encode())
	}

	frame, err := gql.DecodeFrame(payload)
	if err != nil {
		log.Printf("[gql] frame decode error: %v", err)
		return ws.WriteMessage(conn, ipc.MsgGQLResult,
			gql.NewErrorFrame(-101, fmt.Sprintf("invalid frame: %v", err)).Encode())
	}

	// Build session info for ACL validation
	session := &gqlSessionAdapter{sessionCtx: t.sessionCtx}

	query, err := gql.ValidateFrame(frame, session)
	if err != nil {
		// Distinguish syntactic vs semantic errors for proper error codes
		code := int32(-102)
		msg := err.Error()
		switch err.(type) {
		case *gql.SyntacticError:
			code = -102
		case *gql.SemanticError:
			code = -103
		}
		log.Printf("[gql] validation error: %v", err)
		return ws.WriteMessage(conn, ipc.MsgGQLResult,
			gql.NewErrorFrame(code, msg).Encode())
	}

	// Result/Error frames don't have a query (e.g., OpcodeResult)
	if query == nil {
		return ws.WriteMessage(conn, ipc.MsgGQLResult,
			gql.NewErrorFrame(-104, "not a query frame").Encode())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := t.gqlDispatcher.Dispatch(ctx, query)
	if err != nil {
		log.Printf("[gql] dispatch error: %v", err)
		return ws.WriteMessage(conn, ipc.MsgGQLResult,
			gql.NewErrorFrame(-105, fmt.Sprintf("dispatch error: %v", err)).Encode())
	}

	resultFrame := gql.NewResultFrame(result)
	log.Printf("[gql] %s OK (entries=%d total=%d)", query.Operation, len(result.Entries), result.TotalCount)
	return ws.WriteMessage(conn, ipc.MsgGQLResult, resultFrame.Encode())
}

// gqlSessionAdapter wraps security.SessionContext to implement gql.SessionInfo.
type gqlSessionAdapter struct {
	sessionCtx *security.SessionContext
}

func (a *gqlSessionAdapter) IsUnlocked() bool {
	if a.sessionCtx == nil {
		return false
	}
	return a.sessionCtx.IsUnlocked()
}

func (a *gqlSessionAdapter) ActiveHandle() string {
	if a.sessionCtx == nil {
		return ""
	}
	return a.sessionCtx.ActiveHandle()
}

func (a *gqlSessionAdapter) UserID() string {
	return "default"
}

func (a *gqlSessionAdapter) HasRole(role string) bool {
	return role == "default"
}

// wsReconnect handles the MsgReconnect protocol: client sends its session
// token to resume a previous vault session without re-entering the password.
// The daemon validates the token, checks vault state, and if everything matches,
// pushes the full state mirror to the client via MsgStateMirror.
//
// Phase 3: UI-decoupling — Tauri page navigation briefly disconnects/reconnects
// the WebSocket. This handler lets the UI re-attach to an already-unlocked vault
// without prompting the user for their password again.
func (t *Translator) wsReconnect(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.Token == "" {
		return ws.WriteMessage(conn, ipc.MsgSessionResumeErr,
			[]byte(`{"error":"invalid reconnect payload"}`))
	}

	// Validate the reconnection token against the daemon session token.
	if t.tokenValidator != nil && !t.tokenValidator(req.Token) {
		return ws.WriteMessage(conn, ipc.MsgSessionResumeErr,
			[]byte(`{"error":"invalid session token"}`))
	}

	// Check vault state: is it still unlocked?
	unlocked := t.sessionCtx != nil && t.sessionCtx.IsUnlocked()
	if !unlocked {
		return ws.WriteMessage(conn, ipc.MsgSessionResumeErr,
			[]byte(`{"error":"vault locked — re-auth required"}`))
	}

	// Build full state mirror for the reconnected client.
	// Push vault state, workspace info, and entry count so the UI
	// can fully reconstruct its state without additional queries.
	state := map[string]interface{}{
		"unlocked":        true,
		"gate_open":       true,
		"mvk_handle_ok":   t.sessionCtx.MVKHandle() != "",
		"session_resumed": true,
	}

	// Entry count: query block store index (non-blocking, in-memory).
	if t.bus != nil {
		ev := kernel.NewEvent("api", kernel.EvStorageList, nil)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		result, err := t.bus.Request(ctx, ev)
		cancel()
		if err == nil {
			var stored struct {
				Metas []storage.BlockMeta `json:"metas,omitempty"`
			}
			if json.Unmarshal(result.Payload, &stored) == nil {
				state["entry_count"] = len(stored.Metas)
			}
		}
	}

	// Workspace info
	if t.workspaceMgr != nil {
		ws := t.workspaceMgr.Active()
		if ws != nil {
			state["active_workspace"] = map[string]string{
				"id":   ws.ID,
				"name": ws.Name,
			}
		}
	}

	stateJSON, _ := json.Marshal(state)
	log.Printf("[translator:RECONNECT] Session resumed — vault unlocked, entries=%v", state["entry_count"])
	return ws.WriteMessage(conn, ipc.MsgStateMirror, stateJSON)
}

// wsEntryQuery dispatches an ENTRY.QUERY event and streams the filtered entry
// list back to the client as MsgEntryQueryResult.
// Payload: JSON {"category": "PASSWORD"|"SSH_KEY"|"CERTIFICATE"|"FILE_VAULT"|""}
func (t *Translator) wsEntryQuery(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		Category string `json:"category"`
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &req); err != nil {
			return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid entry query payload"))
		}
	}

	evPayload, _ := json.Marshal(map[string]string{"category": req.Category})
	ev := kernel.NewEvent("api", kernel.EvEntryQuery, evPayload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("entry query timeout"))
	}

	log.Printf("[translator:ENTRY_QUERY] category=%q", req.Category)
	return ws.WriteMessage(conn, ipc.MsgEntryQueryResult, result.Payload)
}

// wsSSHKeyGen dispatches a TOOL.SSH_GEN event and returns the generated public
// key + entry ID to the client as MsgSSHKeyResult.
// Payload: JSON {"comment": "user@host", "save_to_vault": true}
func (t *Translator) wsSSHKeyGen(conn *gorillaws.Conn, payload []byte) error {
	ev := kernel.NewEvent("api", kernel.EvToolSSHGen, payload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("SSH key generation timeout"))
	}

	var res struct {
		Error string `json:"error,omitempty"`
	}
	_ = json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(res.Error))
	}

	log.Printf("[translator:SSH_GEN] key generated and saved to vault")
	return ws.WriteMessage(conn, ipc.MsgSSHKeyResult, result.Payload)
}

// wsAuthTokenSubmit validates the token delivered by the UI and, if valid,
// emits KERNEL.STATE_READY to complete the three-way handshake.
func (t *Translator) wsAuthTokenSubmit(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.Token == "" {
		log.Printf("[bridge][handshake] TOKEN rejected: invalid JSON or empty payload")
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid token payload"))
	}

	if t.tokenValidator != nil && !t.tokenValidator(req.Token) {
		log.Printf("[bridge][handshake] TOKEN rejected: mismatch")
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid token"))
	}

	log.Printf("[bridge][handshake] TOKEN validated, emitting STATE_READY")

	initialized, _, _ := grimdb.CheckVaultStatus(t.appDir)
	resp, _ := json.Marshal(map[string]interface{}{
		"status":      "ready",
		"initialized": initialized,
	})
	return ws.WriteMessage(conn, ipc.MsgKernelStateReady, resp)
}

// wsDispatch is a generic helper: fires ev on the kernel bus, waits for a result,
// and writes it back as resultMsg. Any error field in the JSON payload is surfaced
// as MsgError so the UI can display a proper message.
func (t *Translator) wsDispatch(conn *gorillaws.Conn, evType kernel.EventType, payload []byte, resultMsg byte, tag string) error {
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	ev := kernel.NewEvent("api", evType, payload)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		log.Printf("[translator:%s] timeout or dispatch error: %v", tag, err)
		return ws.WriteMessage(conn, ipc.MsgError, []byte(tag+" timeout"))
	}

	var check struct {
		Error string `json:"error,omitempty"`
	}
	_ = json.Unmarshal(result.Payload, &check)
	if check.Error != "" {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(check.Error))
	}

	log.Printf("[translator:%s] ok (%d bytes)", tag, len(result.Payload))
	return ws.WriteMessage(conn, resultMsg, result.Payload)
}
