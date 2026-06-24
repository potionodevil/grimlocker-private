// Package storage (entry_module.go) implementiert EntryHandler — die High-Level-CRUD-Layer,
// die ENTRY.*-Events auf BlockStore-Operationen abbildet.
//
// Anders als der rohe STORAGE-Adapter (der mit opaque Blocks arbeitet), versteht EntryHandler
// VaultEntry-Semantik: Titel, Category, getypte Felder, Timestamps.
// Es ist als direkte Bus-Subscription verdrahtet (kein Module), damit der Caller via
// bus.Request() synchron die ENTRY.RESULT-Reply bekommt.
//
// Unterstützte Events:
//
//	ENTRY.CREATE → erzeugt neuen VaultEntry, weist UUID zu, schreibt in BlockStore
//	ENTRY.READ   → liest rohe Block-Daten anhand der ID
//	ENTRY.UPDATE → liest existierenden Entry, merged Felder, schreibt neu
//	ENTRY.DELETE → entfernt Block (secure delete via BlockStore.DeleteBlock)
//	ENTRY.QUERY  → gibt []BlockMeta gefiltert nach Category aus dem In-Memory-Index zurück
//
// Alle Error-Responses enthalten ein error_code-Feld (aus *errors.GrimlockError),
// damit Caller not-found (2003) von I/O-Fehler (2001) unterscheiden können, ohne
// String-Matching auf dem "error"-Feld.
package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/kernel"
)

// entryHandlerFn ist der interne Handler-Typ für die Entry-Handler-Registry.
type entryHandlerFn func(kernel.Event, kernel.Dispatcher)

// EntryHandler handled ENTRY.*-Events für das Storage.
// Als direkter Handler verdrahtet (kein Module), um synchrones Request/Reply zu unterstützen.
type EntryHandler struct {
	bs         BlockStore
	dispatcher kernel.Dispatcher
	handlers   map[kernel.EventType]entryHandlerFn
}

// NewEntryHandler erzeugt einen EntryHandler für den Bus.
func NewEntryHandler(bs BlockStore) *EntryHandler {
	h := &EntryHandler{bs: bs}
	h.handlers = h.buildHandlers()
	return h
}

// historyBlockID erzeugt die Block-ID für einen History-Snapshot.
// Format: _hist_{entryID}_{unixNano}
func historyBlockID(entryID string, ts int64) string {
	return fmt.Sprintf("_hist_%s_%d", entryID, ts)
}

// isHistoryBlock meldet true, wenn die ID ein History-Snapshot ist (kein echter Entry).
func isHistoryBlock(id string) bool {
	return len(id) > 6 && id[:6] == "_hist_"
}

// buildHandlers gibt die statische Handler-Registry für alle ENTRY.*-Events zurück.
func (h *EntryHandler) buildHandlers() map[kernel.EventType]entryHandlerFn {
	return map[kernel.EventType]entryHandlerFn{
		kernel.EvEntryCreate:  h.handleCreate,
		kernel.EvEntryRead:    h.handleRead,
		kernel.EvEntryUpdate:  h.handleUpdate,
		kernel.EvEntryDelete:  h.handleDelete,
		kernel.EvEntryQuery:   h.handleQuery,
		kernel.EvEntryHistory: h.handleHistory,
		kernel.EvEntryRestore: h.handleRestore,
	}
}

// SetDispatcher setzt den Bus-Dispatcher für Reply-Events.
func (h *EntryHandler) SetDispatcher(d kernel.Dispatcher) {
	h.dispatcher = d
}

// Handle verarbeitet ENTRY.*-Events und dispatche Reply-Events.
// Ist als direkter Handler verdrahtet, nicht als Module.
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

// entryError ist das strukturierte JSON-Schema für ENTRY.RESULT-Error-Responses.
// Enthält error_code, damit Caller vault-locked vs not-found vs IO unterscheiden können.
type entryError struct {
	Error     string `json:"error"`
	ErrorCode int    `json:"error_code,omitempty"`
}

// replyErr dispatche eine strukturierte Error-Reply für das gegebene Event.
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

	// Category auflösen: explizite Category > legacy Type-Mapping.
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
		replyErr(dispatcher, e, err)
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
		replyErr(dispatcher, e, err)
		return
	}

	// Zuerst versuchen, als VaultEntry zu unmarshalen, bei Legacy-Fällen als map.
	var entry VaultEntry
	if err := json.Unmarshal(block.Data, &entry); err == nil && entry.ID != "" {
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

	// Snapshot der alten Version in _history/ speichern, bevor überschrieben wird.
	snapID := historyBlockID(req.ID, time.Now().UnixNano())
	snap := Block{ID: snapID, Data: block.Data, Category: block.Category}
	if snapErr := h.bs.WriteBlock(snap); snapErr != nil {
		log.Printf("[entry:UPDATE:SNAP] failed to snapshot %s: %v", req.ID, snapErr)
		// non-fatal — update proceeds even if snapshot fails
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

	// Cascade-Delete: Wenn das ein FileVault-Manifest ist, lösche erst alle Chunks.
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

// deleteFileVaultIfManifest prüft, ob der Block ein FileVault-Manifest ist, und löscht
// dann alle assoziierten Chunk-Blöcke, bevor das Manifest selbst gelöscht wird.
func (h *EntryHandler) deleteFileVaultIfManifest(id string) error {
	block, err := h.bs.ReadBlock(id)
	if err != nil {
		return nil
	}

	if block.Category != CategoryFileVault {
		return nil
	}

	var manifest BlobManifest
	if err := json.Unmarshal(block.Data, &manifest); err != nil || manifest.ID == "" {
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

// handleHistory listet alle History-Snapshots für eine Entry-ID auf.
// Payload: {"id": "<entry_id>"}
// Response: {"id": "<entry_id>", "snapshots": [{"snap_id","ts","data"},...]}
func (h *EntryHandler) handleHistory(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil || req.ID == "" {
		replyErr(dispatcher, e, gerrors.NewProtocolError("entry_history_unmarshal", err))
		return
	}

	prefix := "_hist_" + req.ID + "_"
	metas, err := h.bs.ListBlocks()
	if err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	type SnapMeta struct {
		SnapID string          `json:"snap_id"`
		Ts     int64           `json:"ts"`
		Data   json.RawMessage `json:"data"`
	}
	var snaps []SnapMeta
	for _, m := range metas {
		if len(m.ID) > len(prefix) && m.ID[:len(prefix)] == prefix {
			b, readErr := h.bs.ReadBlock(m.ID)
			if readErr != nil {
				continue
			}
			var ts int64
			fmt.Sscanf(m.ID[len(prefix):], "%d", &ts)
			snaps = append(snaps, SnapMeta{SnapID: m.ID, Ts: ts, Data: b.Data})
		}
	}
	// Sort newest first by Ts (insertion sort over typically small slice)
	for i := 1; i < len(snaps); i++ {
		for j := i; j > 0 && snaps[j].Ts > snaps[j-1].Ts; j-- {
			snaps[j], snaps[j-1] = snaps[j-1], snaps[j]
		}
	}

	resp, _ := json.Marshal(map[string]interface{}{"id": req.ID, "snapshots": snaps})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, resp)) //nolint:errcheck
}

// handleRestore stellt einen History-Snapshot als aktuellen Entry wieder her.
// Payload: {"id": "<entry_id>", "snap_id": "<_hist_...>"}
func (h *EntryHandler) handleRestore(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		ID     string `json:"id"`
		SnapID string `json:"snap_id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil || req.ID == "" || req.SnapID == "" {
		replyErr(dispatcher, e, gerrors.NewProtocolError("entry_restore_unmarshal", err))
		return
	}

	snap, err := h.bs.ReadBlock(req.SnapID)
	if err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	// Read current block to preserve Category/ID metadata.
	current, err := h.bs.ReadBlock(req.ID)
	if err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	// Snapshot the current version before overwriting.
	snapID := historyBlockID(req.ID, time.Now().UnixNano())
	preSnap := Block{ID: snapID, Data: current.Data, Category: current.Category}
	if snapErr := h.bs.WriteBlock(preSnap); snapErr != nil {
		log.Printf("[entry:RESTORE:SNAP] failed to snapshot current %s: %v", req.ID, snapErr)
	}

	// Restore: write snapshot data as the live block.
	restored := Block{ID: req.ID, Data: snap.Data, Category: current.Category}
	if err := h.bs.WriteBlock(restored); err != nil {
		replyErr(dispatcher, e, err)
		return
	}

	resp, _ := json.Marshal(map[string]interface{}{"status": "restored", "id": req.ID, "snap_id": req.SnapID})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, resp)) //nolint:errcheck
}

// handleQuery verarbeitet ENTRY.QUERY-Events.
// Payload: {"category": "PASSWORD"} — leere Category gibt alle Entries zurück.
// Dispatched ENTRY.RESULT mit einem []BlockMeta-Payload gefiltert nach Category.
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

	// History-Snapshots aus dem Ergebnis herausfiltern — sie sind interne Blöcke.
	filtered := metas[:0]
	for _, m := range metas {
		if !isHistoryBlock(m.ID) {
			filtered = append(filtered, m)
		}
	}
	metas = filtered

	log.Printf("[entry:QUERY] category=%q results=%d", req.Category, len(metas))
	respPayload, _ := json.Marshal(map[string]interface{}{
		"category": req.Category,
		"entries":  metas,
		"count":    len(metas),
	})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
}
