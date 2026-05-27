// Package tools provides the TOOL kernel channel — a set of utility operations
// that generate or derive cryptographic material on behalf of the user.
//
// Current tools:
//   - TOOL.SSH_GEN: generates an Ed25519 SSH key pair and stores it in the vault.
package tools

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/storage"
)

// toolHandlerFn is the internal handler type for the TOOL module.
type toolHandlerFn func(kernel.Event) error

// Module implements kernel.Module for the TOOL channel.
// It handles TOOL.* events and dispatches TOOL.RESULT replies.
type Module struct {
	blockStore storage.BlockStore
	dispatcher kernel.Dispatcher
	handlers   map[kernel.EventType]toolHandlerFn
}

// NewModule creates a tools.Module.
// blockStore is used to persist generated key material as vault entries.
func NewModule(blockStore storage.BlockStore) *Module {
	m := &Module{blockStore: blockStore}
	return m
}

func (m *Module) ID() string         { return "tools" }
func (m *Module) Channels() []string { return []string{"TOOL"} }

func (m *Module) Start(ctx context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	m.handlers = m.buildHandlers()
	log.Printf("[tools] Module started — handlers: TOOL.SSH_GEN")
	return nil
}

func (m *Module) Stop() error { return nil }

// buildHandlers returns the static handler registry for all TOOL.* events.
func (m *Module) buildHandlers() map[kernel.EventType]toolHandlerFn {
	return map[kernel.EventType]toolHandlerFn{
		kernel.EvToolSSHGen: m.handleSSHGen,
		// No-op: reply events reach all channel subscribers but we don't need to handle them.
		kernel.EvToolResult: func(kernel.Event) error { return nil },
	}
}

// Handle dispatches the event to the registered handler.
// Unknown TOOL.* events are logged at DEBUG level without returning an error.
func (m *Module) Handle(e kernel.Event) error {
	if h, ok := m.handlers[e.Type]; ok {
		return h(e)
	}
	log.Printf("[bus][DEBUG] module=%s no_handler event=%s origin=%s", m.ID(), e.Type, e.Origin)
	return nil
}

// handleSSHGen processes a TOOL.SSH_GEN event.
//
// Request payload (JSON):
//
//	{"comment": "user@host", "save_to_vault": true}
//
// Response (TOOL.RESULT payload, JSON):
//
//	{"public_key": "ssh-ed25519 AAAA… comment", "fingerprint": "SHA256:…", "entry_id": "uuid"}
func (m *Module) handleSSHGen(e kernel.Event) error {
	var req struct {
		Comment     string `json:"comment"`
		SaveToVault bool   `json:"save_to_vault"`
	}
	// Default: save to vault.
	req.SaveToVault = true

	if len(e.Payload) > 0 {
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			return m.replyError(e, fmt.Errorf("invalid SSH_GEN request: %w", err))
		}
	}

	if req.Comment == "" {
		req.Comment = "grimlocker-generated"
	}

	log.Printf("[tools:SSH_GEN] generating Ed25519 key pair comment=%q saveToVault=%v",
		req.Comment, req.SaveToVault)

	pair, err := GenerateEd25519Pair(req.Comment)
	if err != nil {
		return m.replyError(e, fmt.Errorf("key generation failed: %w", err))
	}

	var entryID string

	if req.SaveToVault && m.blockStore != nil {
		// Build a VaultEntry for the SSH key.
		now := time.Now().UnixNano()
		entryID = newUUID()

		entry := storage.VaultEntry{
			ID:       entryID,
			Title:    req.Comment,
			Category: storage.CategorySSHKey,
			Type:     "ssh",
			Fields: map[string]string{
				"public_key":  pair.PublicKey,
				"fingerprint": pair.Fingerprint,
				"comment":     pair.Comment,
				// private_key_pem lives in the block Data (storedEntry below),
				// not in Fields, so it never appears in listing metadata.
			},
			CreatedAt: now,
			UpdatedAt: now,
		}

		// storedEntry extends VaultEntry with the private key PEM so it is
		// encrypted at rest inside the block.Data. It is only decrypted on an
		// explicit "Reveal" action.
		type storedEntry struct {
			storage.VaultEntry
			PrivateKeyPEM string `json:"private_key_pem"`
		}
		storedFull := storedEntry{
			VaultEntry:    entry,
			PrivateKeyPEM: string(pair.PrivateKeyPEM),
		}
		storedJSON, _ := json.Marshal(storedFull)

		block := storage.Block{
			ID:       entryID,
			Data:     storedJSON,
			Category: storage.CategorySSHKey,
		}

		if err := m.blockStore.WriteBlock(block); err != nil {
			log.Printf("[tools:SSH_GEN] vault save failed: %v", err)
			// Non-fatal: still reply with the generated key.
			entryID = ""
		} else {
			log.Printf("[tools:SSH_GEN] saved as entryID=%s", entryID)
		}
		pair.EntryID = entryID
	}

	result, _ := json.Marshal(map[string]string{
		"public_key":  pair.PublicKey,
		"fingerprint": pair.Fingerprint,
		"entry_id":    entryID,
		"comment":     pair.Comment,
	})
	reply := kernel.ReplyEvent(m.ID(), kernel.EvToolResult, e, result)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) replyError(req kernel.Event, err error) error {
	log.Printf("[tools] handler error: %v", err)
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	reply := kernel.ReplyEvent(m.ID(), kernel.EvToolResult, req, payload)
	if dErr := m.dispatcher.Dispatch(reply); dErr != nil {
		return fmt.Errorf("%w (reply dispatch: %v)", err, dErr)
	}
	return err
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
