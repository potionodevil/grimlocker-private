// Package storage (entry_module.go) implements EntryHandler — the high-level
// CRUD layer that maps ENTRY.* events to BlockStore operations.
//
// Unlike the raw STORAGE adapter (which works with opaque Blocks), EntryHandler
// understands VaultEntry semantics: titles, categories, typed fields, timestamps.
// It is wired as a direct bus subscription (not a Module) so the caller's
// bus.Request() can receive the ENTRY.RESULT reply synchronously.
//
// Supported events:
//
//	ENTRY.CREATE → creates a new VaultEntry, assigns UUID, writes to BlockStore
//	ENTRY.READ   → reads raw block data by ID
//	ENTRY.UPDATE → reads existing entry, merges fields, re-writes block
//	ENTRY.DELETE → removes block (secure delete via BlockStore.DeleteBlock)
//	ENTRY.QUERY  → returns []BlockMeta filtered by Category from in-memory index
//
// All error replies include an error_code field (from *errors.GrimlockError)
// so callers can distinguish not-found (2003) from I/O failure (2001) without
// string matching on the "error" field.
package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	gerrors "github.com/grimlocker/grimdb/errors"
	"github.com/grimlocker/grimdb/kernel"
)

// entryHandlerFn is the internal handler type for the entry handler registry.
type entryHandlerFn func(kernel.Event, kernel.Dispatcher)

// EntryHandler handles ENTRY.* events for storage.
// Wired as a direct handler (not a Module) to support synchronous Request/Reply.
type EntryHandler struct {
	bs         BlockStore
	dispatcher kernel.Dispatcher
	handlers   map[kernel.EventType]entryHandlerFn
}

// NewEntryHandler creates an EntryHandler for the bus.
func NewEntryHandler(bs BlockStore) *EntryHandler {
	h := &EntryHandler{bs: bs}
	h.handlers = h.buildHandlers()
	return h
}

// buildHandlers returns the static handler registry for all ENTRY.* events.
func (h *EntryHandler) buildHandlers() map[kernel.EventType]entryHandlerFn {
	return map[kernel.EventType]entryHandlerFn{
		kernel.EvEntryCreate: h.handleCreate,
		kernel.EvEntryRead:   h.handleRead,
		kernel.EvEntryUpdate: h.handleUpdate,
		kernel.EvEntryDelete: h.handleDelete,
		kernel.EvEntryQuery:  h.handleQuery,
	}
}

// SetDispatcher sets the bus dispatcher for sending reply events.
func (h *EntryHandler) SetDispatcher(d kernel.Dispatcher) {
	h.dispatcher = d
}

// Handle processes ENTRY.* events and dispatches reply events.
// This is wired as a direct handler, not as a Module.
func (h *EntryHandler) Handle(e kernel.Event) error {
	if h.dispatcher == nil {
		return fmt.Errorf("dispatcher not initialized")
	}
	if fn, ok := h.handlers[e.Type]; ok {
		fn(e, h.dispatcher)
		return nil
	}
	log.Printf("[bus][DEBUG] module=entry no_handler event=%s origin=%s", e.Type, e.Origin)
	return nil
}

// entryError is the structured JSON schema for ENTRY.RESULT error replies.
// Includes error_code so callers can distinguish vault-locked vs not-found vs IO.
type entryError struct {
	Error     string `json:"error"`
	ErrorCode int    `json:"error_code,omitempty"`
}

// replyErr dispatches a structured error reply for the given event.
func replyErr(dispatcher kernel.Dispatcher, e kernel.Event, err error) {
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	payload, _ := json.Marshal(entryError{Error: err.Error(), ErrorCode: code})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, payload)) //nolint:errcheck
}

func (h *EntryHandler) handleCreate(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		SubjectID string            `json:"subject_id"`
		Title     string            `json:"title"`
		Type      string            `json:"type"`
		Category  string            `json:"category"` // preferred field; falls back to Type
		Fields    map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		replyErr(dispatcher, e, gerrors.NewProtocolError("entry_create_unmarshal", err))
		return
	}

	// Resolve Category: explicit category > legacy type mapping
	cat := Category(req.Category)
	if cat == "" {
		cat = CategoryFromType(req.Type)
	}
	if req.Type == "" && cat != "" {
		req.Type = string(cat)
	}

	now := time.Now().UnixNano()
	entryID := generateUUID()

	entry := VaultEntry{
		ID:        entryID,
		Title:     req.Title,
		Category:  cat,
		Type:      req.Type,
		Fields:    req.Fields,
		SubjectID: req.SubjectID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	entryData, _ := json.Marshal(entry)

	block := Block{
		ID:       entryID,
		Data:     entryData,
		Category: cat,
	}

	if err := h.bs.WriteBlock(block); err != nil {
		log.Printf("[entry:CREATE:FAIL] entryID=%s err=%v", entryID, err)
		replyErr(dispatcher, e, err)
		return
	}

	log.Printf("[entry:CREATE:OK] entryID=%s title=%q category=%s", entryID, req.Title, cat)
	respPayload, _ := json.Marshal(entry)
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
}

func (h *EntryHandler) handleRead(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		replyErr(dispatcher, e, gerrors.NewProtocolError("entry_read_unmarshal", err))
		return
	}

	block, err := h.bs.ReadBlock(req.ID)
	if err != nil {
		replyErr(dispatcher, e, err) // already *GrimlockError (StorageNotFound etc.)
		return
	}

	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, block.Data)) //nolint:errcheck
}

func (h *EntryHandler) handleUpdate(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		SubjectID string            `json:"subject_id"`
		ID        string            `json:"id"`
		Title     string            `json:"title"`
		Fields    map[string]string `json:"fields"`
		Category  string            `json:"category,omitempty"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		replyErr(dispatcher, e, gerrors.NewProtocolError("entry_update_unmarshal", err))
		return
	}

	block, err := h.bs.ReadBlock(req.ID)
	if err != nil {
		replyErr(dispatcher, e, err) // *GrimlockError (StorageNotFound)
		return
	}

	// Try to unmarshal as a VaultEntry first, fall back to map for legacy entries.
	var entry VaultEntry
	if err := json.Unmarshal(block.Data, &entry); err == nil && entry.ID != "" {
		// VaultEntry path: update structured fields.
		entry.Title = req.Title
		entry.Fields = req.Fields
		if req.Category != "" {
			entry.Category = Category(req.Category)
		}
		entry.UpdatedAt = time.Now().UnixNano()

		updatedData, _ := json.Marshal(entry)
		block.Data = updatedData
		block.Category = entry.Category
	} else {
		// Legacy map path: preserve existing structure.
		var existing map[string]interface{}
		if err := json.Unmarshal(block.Data, &existing); err != nil {
			replyErr(dispatcher, e, gerrors.NewStorageCorruptionError("entry_update_unmarshal_legacy", req.ID, nil))
			return
		}
		existing["title"] = req.Title
		existing["fields"] = req.Fields
		updatedData, _ := json.Marshal(existing)
		block.Data = updatedData
	}

	if err := h.bs.WriteBlock(block); err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, block.Data)) //nolint:errcheck
}

func (h *EntryHandler) handleDelete(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		replyErr(dispatcher, e, gerrors.NewProtocolError("entry_delete_unmarshal", err))
		return
	}

	// Cascade-delete: if this is a FileVault manifest, delete all chunk blocks first.
	if err := h.deleteFileVaultIfManifest(req.ID); err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	if err := h.bs.DeleteBlock(req.ID); err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	respPayload, _ := json.Marshal(map[string]string{"status": "deleted"})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload)) //nolint:errcheck
}

// deleteFileVaultIfManifest checks if the block is a FileVault manifest and deletes
// all associated chunk blocks before the manifest itself is deleted.
func (h *EntryHandler) deleteFileVaultIfManifest(id string) error {
	block, err := h.bs.ReadBlock(id)
	if err != nil {
		// Not found — caller will handle the error on the manifest deletion attempt.
		return nil
	}

	if block.Category != CategoryFileVault {
		return nil
	}

	var manifest BlobManifest
	if err := json.Unmarshal(block.Data, &manifest); err != nil || manifest.ID == "" {
		// Not a manifest (could be a chunk block or legacy format). Continue.
		return nil
	}

	for _, chunkID := range manifest.ChunkIDs {
		if delErr := h.bs.DeleteBlock(chunkID); delErr != nil {
			log.Printf("[entry:DELETE:CASCADE] failed to delete chunk %s: %v", chunkID, delErr)
		}
	}

	log.Printf("[entry:DELETE:CASCADE] removed %d chunks for manifest %s", len(manifest.ChunkIDs), id)
	return nil
}

// handleQuery handles ENTRY.QUERY events.
// Payload: {"category": "PASSWORD"} — empty category returns all entries.
// Dispatches ENTRY.RESULT with a []BlockMeta payload filtered by category.
func (h *EntryHandler) handleQuery(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		Category string `json:"category"`
	}
	if len(e.Payload) > 0 {
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			respPayload, _ := json.Marshal(map[string]string{"error": "invalid query request"})
			dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
			return
		}
	}

	metas, err := h.bs.QueryBlocks(Category(req.Category))
	if err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	log.Printf("[entry:QUERY] category=%q results=%d", req.Category, len(metas))
	respPayload, _ := json.Marshal(map[string]interface{}{
		"category": req.Category,
		"entries":  metas,
		"count":    len(metas),
	})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
}
