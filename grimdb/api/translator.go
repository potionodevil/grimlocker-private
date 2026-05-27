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
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/api/handlers"
	"github.com/grimlocker/grimdb/api/ipc"
	ws "github.com/grimlocker/grimdb/api/websocket"
	rustbridge "github.com/grimlocker/grimdb/cgo"
	"github.com/grimlocker/grimdb/crypto"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/storage"
	"github.com/grimlocker/grimdb/storage/grimdb"
	"golang.org/x/crypto/chacha20poly1305"
)

const requestTimeout = 30 * time.Second

// Translator converts binary IPC frames into kernel Events and delivers
// kernel result Events back as binary frames.
type Translator struct {
	bus          kernel.Dispatcher
	db           *grimdb.GrimDB
	appDir       string
	crypto       crypto.Provider
	bridge       *ws.Bridge
	entryHandler *handlers.EntryHandler
	workspaceMgr *storage.WorkspaceManager

	// tokenValidator is injected by the daemon to validate AUTH.TOKEN_SUBMIT payloads.
	tokenValidator func(string) bool

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
}

// NewTranslator creates a Translator.
func NewTranslator(bus kernel.Dispatcher, db *grimdb.GrimDB, appDir string, cryptoProv crypto.Provider, workspaceMgr *storage.WorkspaceManager) *Translator {
	return &Translator{
		bus:          bus,
		db:           db,
		appDir:       appDir,
		crypto:       cryptoProv,
		workspaceMgr: workspaceMgr,
	}
}

// HandshakeStatus emits the kernel state report on initial WebSocket connection.
// Reports whether the vault is initialized so the UI can decide the next action.
func (t *Translator) HandshakeStatus(conn *gorillaws.Conn) error {
	initialized, _, _ := grimdb.CheckVaultStatus(t.appDir)

	payload, _ := json.Marshal(map[string]interface{}{
		"status":      "Online",
		"initialized": initialized,
	})
	return ws.WriteMessage(conn, ipc.MsgAck, payload)
}

// SetEntryHandler injects the entry handler for CRUD operations.
func (t *Translator) SetEntryHandler(eh *handlers.EntryHandler) {
	t.entryHandler = eh
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

// SetMVKResolver injects a function that returns the current MVK from locked memory.
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

// SetTokenValidator injects the function used to validate AUTH.TOKEN_SUBMIT payloads.
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

	case ipc.MsgAuthTokenSubmit:
		return t.wsAuthTokenSubmit(conn, payload)

	case ipc.MsgEntryQuery:
		return t.wsEntryQuery(conn, payload)

	case ipc.MsgSSHKeyGen:
		return t.wsSSHKeyGen(conn, payload)

	case ipc.MsgSystemHeartbeat:
		// Client acknowledged heartbeat — no action needed.
		return nil

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
	password := string(payload)

	evPayload, _ := json.Marshal(map[string]string{"password": password, "app_dir": t.appDir})
	ev := kernel.NewEvent("api", kernel.EvAuthUnlock, evPayload)

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := t.bus.Request(ctx, ev)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("unlock timeout"))
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte(reason))
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
	// Frontend sends {type, data} — transform into encrypted Block.
	var raw struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid entry payload"))
	}

	entryID := generateID()
	entry := map[string]interface{}{
		"id":   entryID,
		"type": raw.Type,
		"data": raw.Data,
	}
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("marshal entry failed"))
	}

	// Encrypt the entry JSON with the master vault key.
	mk := t.mvkResolver()
	if mk == nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("vault locked: no key available"))
	}
	nonce, err := t.crypto.NewNonce()
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("nonce generation failed"))
	}
	ct, err := t.crypto.Encrypt(mk, nonce[:], entryJSON, nil)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("encryption failed"))
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("save timeout"))
	}
	var res struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(res.Error))
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("list timeout"))
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
		Title     string `json:"title,omitempty"`
		CreatedAt int64  `json:"created_at"`
		UpdatedAt int64  `json:"updated_at"`
	}
	summaries := make([]entrySummary, 0, len(stored.Metas))
	mk := t.mvkResolver()

	for _, meta := range stored.Metas {
		sum := entrySummary{
			ID:        meta.ID,
			CreatedAt: meta.CreatedAt,
			UpdatedAt: meta.UpdatedAt,
		}
		if mk != nil && meta.ID != "" {
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
				if rd.Block != nil && len(rd.Block.Data) >= 12 {
					pt, decErr := t.crypto.Decrypt(mk, rd.Block.Data[:12], rd.Block.Data[12:], nil)
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("get timeout"))
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("vault locked: no key available"))
	}

	blob := stored.Block.Data
	if len(blob) < 12 {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("corrupt entry: blob too short"))
	}

	pt, err := t.crypto.Decrypt(mk, blob[:12], blob[12:], nil)
	if err != nil {
		log.Printf("[translator:GET] decrypt FAIL entryID=%s: %v", string(payload), err)
		return ws.WriteMessage(conn, ipc.MsgError, []byte("decryption failed"))
	}

	var fullEntry struct {
		ID   string          `json:"id"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(pt, &fullEntry) != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid entry format"))
	}

	metadata := map[string]interface{}{
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("decrypt timeout"))
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("vault locked: no key available"))
	}

	blob := stored.Block.Data
	if len(blob) < 12 {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("corrupt entry: blob too short"))
	}

	pt, err := t.crypto.Decrypt(mk, blob[:12], blob[12:], nil)
	if err != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("decryption failed"))
	}

	var fullEntry struct {
		ID   string          `json:"id"`
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(pt, &fullEntry) != nil {
		return ws.WriteMessage(conn, ipc.MsgError, []byte("invalid entry format"))
	}

	decryptedResult := map[string]interface{}{
		"id":         stored.Block.ID,
		"type":       fullEntry.Type,
		"data":       fullEntry.Data,
		"created_at": stored.Block.CreatedAt,
		"updated_at": stored.Block.UpdatedAt,
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
		return ws.WriteMessage(conn, ipc.MsgError, []byte("delete timeout"))
	}

	var res struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		return ws.WriteMessage(conn, ipc.MsgError, []byte(res.Error))
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
		entropyPath = req.EntropyPath
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

	// If a session key is available, SKE-encrypt the recovery phrase before
	// sending it over the wire. Otherwise, the connection is localhost-only
	// (Tauri sidecar), so plaintext is acceptable but logged as a warning.
	if t.sessionKeySet {
		encData, encErr := t.skeEncrypt([]byte(phrase))
		if encErr == nil {
			resp, _ := json.Marshal(map[string]string{"encrypted": encData})
			return ws.WriteMessage(conn, ipc.MsgRecoveryPhraseData, resp)
		}
		log.Printf("[translator:RECOVERY] SKE encrypt failed, sending plaintext: %v", encErr)
	}

	return ws.WriteMessage(conn, ipc.MsgRecoveryPhraseData, []byte(phrase))
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

	// Also dispatch AUTH.LOGOUT to trigger re-login for the new workspace
	logoutEv := kernel.NewEvent("api", kernel.EvAuthLogout, []byte(`{"reason":"workspace_switched"}`))
	_ = t.bus.Dispatch(logoutEv)

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
