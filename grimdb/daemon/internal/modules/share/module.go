// Package share implementiert das SHARE-Kanal-Module für sichere, einmalige Entry-Freigaben.
//
// Idee: Ein Vault-Eintrag wird mit einem ephemeren ChaCha20-Key verschlüsselt.
// Der Key + UUID + ExpiresAt ergeben zusammen den "Share-Token" (URL-safe Base64).
// Der Empfänger gibt den Token ein; der Daemon entschlüsselt den Blob und gibt den Entry zurück.
// Jeder Share kann nur einmal eingelöst werden (OneTime=true) oder mehrfach bis TTL.
//
// Unterstützte Events:
//
//	SHARE.CREATE → CreateRequest → ShareResult{token, expires_at}
//	SHARE.REDEEM → RedeemRequest{token} → ShareResult{entry_json}
//	SHARE.REVOKE → RevokeRequest{share_id} → ShareResult{status}
package share

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
)

// shareEntry ist eine im Memory gehaltene Share-Session.
type shareEntry struct {
	ID         string
	EncPayload []byte    // ChaCha20-Poly1305 verschlüsselter Entry
	Key        []byte    // 32-Byte Ephemeral-Key
	Nonce      []byte    // 12-Byte Nonce
	ExpiresAt  time.Time
	OneTime    bool
	Redeemed   bool
}

// Module implementiert das SHARE-Kanal-Module.
type Module struct {
	dispatcher kernel.Dispatcher
	store      storage.BlockStore
	mu         sync.RWMutex
	shares     map[string]*shareEntry // share_id → entry
}

// NewModule erzeugt ein neues Share-Module.
func NewModule(store storage.BlockStore) *Module {
	return &Module{
		store:  store,
		shares: make(map[string]*shareEntry),
	}
}

func (m *Module) ID() string         { return "share" }
func (m *Module) Channels() []string { return []string{"SHARE"} }

func (m *Module) Start(_ context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	go m.gcLoop()
	log.Println("[share] module started")
	return nil
}

func (m *Module) Stop() error {
	log.Println("[share] module stopped")
	return nil
}

func (m *Module) Handle(e kernel.Event) error {
	switch e.Type {
	case kernel.EvShareCreate:
		return m.handleCreate(e)
	case kernel.EvShareRedeem:
		return m.handleRedeem(e)
	case kernel.EvShareRevoke:
		return m.handleRevoke(e)
	default:
		return nil
	}
}

// ─── Create ───────────────────────────────────────────────────────────────────

type createRequest struct {
	EntryID  string `json:"entry_id"`
	TTLHours int    `json:"ttl_hours"` // default 24
	OneTime  bool   `json:"one_time"`  // einmalig (default true)
}

type createResult struct {
	ShareID   string `json:"share_id"`
	Token     string `json:"token"`      // URL-safe base64 {shareID}:{keyHex}
	ExpiresAt int64  `json:"expires_at"` // Unix-Timestamp
	Error     string `json:"error,omitempty"`
}

func (m *Module) handleCreate(e kernel.Event) error {
	var req createRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil || req.EntryID == "" {
		return m.replyError(e, fmt.Errorf("share.create: unmarshal: %w", err))
	}

	block, err := m.store.ReadBlock(req.EntryID)
	if err != nil {
		return m.replyError(e, fmt.Errorf("share.create: read entry: %w", err))
	}

	ttl := req.TTLHours
	if ttl <= 0 { ttl = 24 }

	// Ephemeral-Key + Nonce generieren
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return m.replyError(e, fmt.Errorf("share.create: rand key: %w", err))
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return m.replyError(e, fmt.Errorf("share.create: rand nonce: %w", err))
	}

	// Share-ID
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return m.replyError(e, fmt.Errorf("share.create: rand id: %w", err))
	}
	shareID := base64.RawURLEncoding.EncodeToString(idBytes)

	// Entry-Daten verschlüsseln (block.Data enthält den Entry-JSON)
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return m.replyError(e, fmt.Errorf("share.create: aead: %w", err))
	}
	encPayload := aead.Seal(nil, nonce, block.Data, nil)

	expiresAt := time.Now().Add(time.Duration(ttl) * time.Hour)

	m.mu.Lock()
	m.shares[shareID] = &shareEntry{
		ID:         shareID,
		EncPayload: encPayload,
		Key:        key,
		Nonce:      nonce,
		ExpiresAt:  expiresAt,
		OneTime:    req.OneTime || true, // default one-time
	}
	m.mu.Unlock()

	// Token = base64(shareID + ":" + base64(key))
	tokenData := shareID + ":" + base64.RawURLEncoding.EncodeToString(key)
	token := base64.RawURLEncoding.EncodeToString([]byte(tokenData))

	payload, _ := json.Marshal(createResult{
		ShareID:   shareID,
		Token:     "grimshare://" + token,
		ExpiresAt: expiresAt.Unix(),
	})
	m.dispatcher.Dispatch(kernel.ReplyEvent("share", kernel.EvShareResult, e, payload)) //nolint:errcheck
	log.Printf("[share:CREATE] id=%s ttl=%dh one_time=%v", shareID, ttl, req.OneTime)
	return nil
}

// ─── Redeem ───────────────────────────────────────────────────────────────────

type redeemRequest struct {
	Token string `json:"token"`
}

type redeemResult struct {
	EntryJSON string `json:"entry_json,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (m *Module) handleRedeem(e kernel.Event) error {
	var req redeemRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil || req.Token == "" {
		return m.replyError(e, fmt.Errorf("share.redeem: unmarshal: %w", err))
	}

	shareID, key, err := parseToken(req.Token)
	if err != nil {
		return m.replyError(e, fmt.Errorf("share.redeem: invalid token: %w", err))
	}

	m.mu.Lock()
	sh, ok := m.shares[shareID]
	if !ok {
		m.mu.Unlock()
		return m.replyError(e, fmt.Errorf("share.redeem: share not found or expired"))
	}
	if sh.Redeemed {
		m.mu.Unlock()
		return m.replyError(e, fmt.Errorf("share.redeem: already redeemed"))
	}
	if time.Now().After(sh.ExpiresAt) {
		delete(m.shares, shareID)
		m.mu.Unlock()
		return m.replyError(e, fmt.Errorf("share.redeem: expired"))
	}
	if sh.OneTime {
		sh.Redeemed = true
	}
	encPayload := sh.EncPayload
	nonce := sh.Nonce
	m.mu.Unlock()

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return m.replyError(e, fmt.Errorf("share.redeem: aead: %w", err))
	}
	plain, err := aead.Open(nil, nonce, encPayload, nil)
	if err != nil {
		return m.replyError(e, fmt.Errorf("share.redeem: decrypt: %w", err))
	}

	payload, _ := json.Marshal(redeemResult{EntryJSON: string(plain)})
	m.dispatcher.Dispatch(kernel.ReplyEvent("share", kernel.EvShareResult, e, payload)) //nolint:errcheck
	log.Printf("[share:REDEEM] id=%s", shareID)
	return nil
}

// ─── Revoke ───────────────────────────────────────────────────────────────────

type revokeRequest struct {
	ShareID string `json:"share_id"`
}

func (m *Module) handleRevoke(e kernel.Event) error {
	var req revokeRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil || req.ShareID == "" {
		return m.replyError(e, fmt.Errorf("share.revoke: unmarshal: %w", err))
	}

	m.mu.Lock()
	_, existed := m.shares[req.ShareID]
	delete(m.shares, req.ShareID)
	m.mu.Unlock()

	status := "revoked"
	if !existed {
		status = "not_found"
	}
	payload, _ := json.Marshal(map[string]string{"status": status, "share_id": req.ShareID})
	m.dispatcher.Dispatch(kernel.ReplyEvent("share", kernel.EvShareResult, e, payload)) //nolint:errcheck
	log.Printf("[share:REVOKE] id=%s existed=%v", req.ShareID, existed)
	return nil
}

// ─── GC ───────────────────────────────────────────────────────────────────────

func (m *Module) gcLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		m.mu.Lock()
		for id, sh := range m.shares {
			if now.After(sh.ExpiresAt) || sh.Redeemed {
				delete(m.shares, id)
			}
		}
		m.mu.Unlock()
	}
}

// ─── Token helpers ────────────────────────────────────────────────────────────

func parseToken(raw string) (shareID string, key []byte, err error) {
	// Strip "grimshare://" prefix if present.
	const prefix = "grimshare://"
	if len(raw) > len(prefix) && raw[:len(prefix)] == prefix {
		raw = raw[len(prefix):]
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", nil, fmt.Errorf("base64 decode: %w", err)
	}
	// Format: "<shareID>:<base64key>"
	sep := -1
	s := string(decoded)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' { sep = i; break }
	}
	if sep < 0 {
		return "", nil, fmt.Errorf("invalid token format")
	}
	shareID = s[:sep]
	key, err = base64.RawURLEncoding.DecodeString(s[sep+1:])
	if err != nil {
		return "", nil, fmt.Errorf("key decode: %w", err)
	}
	if len(key) != 32 {
		return "", nil, fmt.Errorf("invalid key length %d", len(key))
	}
	return shareID, key, nil
}

// ─── Error helper ─────────────────────────────────────────────────────────────

func (m *Module) replyError(e kernel.Event, err error) error {
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	m.dispatcher.Dispatch(kernel.ReplyEvent("share", kernel.EvShareResult, e, payload)) //nolint:errcheck
	return err
}
