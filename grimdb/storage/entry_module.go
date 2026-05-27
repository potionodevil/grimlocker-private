package storage

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/grimlocker/grimdb/kernel"
)

// EntryHandler handles ENTRY.* events for storage.
// Wired as a direct handler (not a Module) to support synchronous Request/Reply.
type EntryHandler struct {
	bs         BlockStore
	dispatcher kernel.Dispatcher
}

// NewEntryHandler creates an EntryHandler for the bus.
func NewEntryHandler(bs BlockStore) *EntryHandler {
	return &EntryHandler{bs: bs}
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
	switch e.Type {
	case kernel.EvEntryCreate:
		h.handleCreate(e, h.dispatcher)
	case kernel.EvEntryRead:
		h.handleRead(e, h.dispatcher)
	case kernel.EvEntryUpdate:
		h.handleUpdate(e, h.dispatcher)
	case kernel.EvEntryDelete:
		h.handleDelete(e, h.dispatcher)
	default:
		return fmt.Errorf("unknown entry event: %s", e.Type)
	}
	return nil
}

func (h *EntryHandler) handleCreate(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		SubjectID string            `json:"subject_id"`
		Title     string            `json:"title"`
		Type      string            `json:"type"`
		Fields    map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "invalid request"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	entryID := generateUUID()
	entry := map[string]interface{}{
		"id":         entryID,
		"title":      req.Title,
		"type":       req.Type,
		"fields":     req.Fields,
		"subject_id": req.SubjectID,
	}
	entryData, _ := json.Marshal(entry)

	block := Block{
		ID:   entryID,
		Data: entryData,
	}

	if err := h.bs.WriteBlock(block); err != nil {
		log.Printf("[entry:CREATE:FAIL] entryID=%s err=%v", entryID, err)
		respPayload, _ := json.Marshal(map[string]string{"error": err.Error()})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	log.Printf("[entry:CREATE:OK] entryID=%s title=%q", entryID, req.Title)
	respPayload, _ := json.Marshal(entry)
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
}

func (h *EntryHandler) handleRead(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "invalid request"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	block, err := h.bs.ReadBlock(req.ID)
	if err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "entry not found"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, block.Data))
}

func (h *EntryHandler) handleUpdate(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		SubjectID string            `json:"subject_id"`
		ID        string            `json:"id"`
		Title     string            `json:"title"`
		Fields    map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "invalid request"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	block, err := h.bs.ReadBlock(req.ID)
	if err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "entry not found"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	var existing map[string]interface{}
	if err := json.Unmarshal(block.Data, &existing); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "corrupt entry"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	existing["title"] = req.Title
	existing["fields"] = req.Fields

	updatedData, _ := json.Marshal(existing)
	block.Data = updatedData

	if err := h.bs.WriteBlock(block); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": err.Error()})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	respPayload, _ := json.Marshal(existing)
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
}

func (h *EntryHandler) handleDelete(e kernel.Event, dispatcher kernel.Dispatcher) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": "invalid request"})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	if err := h.bs.DeleteBlock(req.ID); err != nil {
		respPayload, _ := json.Marshal(map[string]string{"error": err.Error()})
		dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
		return
	}

	respPayload, _ := json.Marshal(map[string]string{"status": "deleted"})
	dispatcher.Dispatch(kernel.ReplyEvent("storage", kernel.EvEntryResult, e, respPayload))
}
