package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/api/ipc"
	"github.com/grimlocker/grimdb/api/websocket"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/storage"
)

const requestTimeout = 30 * time.Second

type activeIngest struct {
	pw       *io.PipeWriter
	manifest chan storage.BlobManifest
	err      chan error
}

// EntryHandler manages CRUD operations and file streaming for vault entries.
type EntryHandler struct {
	bus           kernel.Dispatcher
	bridge        *websocket.Bridge
	policy        *PolicyManager
	ingest        *storage.IngestEngine
	retrieveMVK   func(string) ([]byte, bool) // security.Module.RetrieveMVK
	mu            sync.Mutex
	mvkHandle     string // Current handle after unlock
	activeIngests map[*gorillaws.Conn]*activeIngest
}

// NewEntryHandler creates an EntryHandler.
func NewEntryHandler(
	bus kernel.Dispatcher,
	bridge *websocket.Bridge,
	policy *PolicyManager,
	ingestEngine *storage.IngestEngine,
	retrieveMVK func(string) ([]byte, bool),
) *EntryHandler {
	return &EntryHandler{
		bus:           bus,
		bridge:        bridge,
		policy:        policy,
		ingest:        ingestEngine,
		retrieveMVK:   retrieveMVK,
		activeIngests: make(map[*gorillaws.Conn]*activeIngest),
	}
}

// SetMVKHandle sets the current MVK handle (called after successful AUTH.UNLOCK).
func (h *EntryHandler) SetMVKHandle(handle string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mvkHandle = handle
}

// getMVK retrieves the current MVK from the handle.
func (h *EntryHandler) getMVK() ([]byte, bool) {
	h.mu.Lock()
	handle := h.mvkHandle
	h.mu.Unlock()
	if handle == "" {
		return nil, false
	}
	return h.retrieveMVK(handle)
}

// SubscribeToAuthResult subscribes to AUTH.RESULT events and extracts the MVK handle.
func (h *EntryHandler) SubscribeToAuthResult() {
	h.bus.Subscribe(kernel.EvAuthResult, func(e kernel.Event) error {
		var res struct {
			Success   bool   `json:"success"`
			MVKHandle string `json:"mvk_handle"`
		}
		if err := json.Unmarshal(e.Payload, &res); err != nil {
			log.Printf("[EntryHandler:AUTH.RESULT] unmarshal error: %v (raw: %s)", err, string(e.Payload))
			return nil
		}
		log.Printf("[EntryHandler:AUTH.RESULT] success=%v mvk_handle=%s ReplyTo=%s",
			res.Success, res.MVKHandle, e.ReplyTo)
		if res.Success && res.MVKHandle != "" {
			h.SetMVKHandle(res.MVKHandle)
		}
		return nil
	})
}

// HandleCreate processes an entry creation request.
func (h *EntryHandler) HandleCreate(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		SubjectID string            `json:"subject_id"`
		Title     string            `json:"title"`
		Type      string            `json:"type"`
		Fields    map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}
	if req.SubjectID == "" {
		req.SubjectID = "default"
	}

	if !h.policy.CheckWrite(req.SubjectID) {
		h.policy.OnUnauthorized(req.SubjectID, "ENTRY.CREATE")
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("unauthorized"))
	}

	// Dispatch ENTRY.CREATE event
	ev := kernel.NewEvent("api", kernel.EvEntryCreate, payload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := h.bus.Request(ctx, ev)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("request timeout"))
	}

	return websocket.WriteMessage(conn, ipc.MsgEntryResult, result.Payload)
}

// HandleRead processes an entry read request.
func (h *EntryHandler) HandleRead(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		SubjectID string `json:"subject_id"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}
	if req.SubjectID == "" {
		req.SubjectID = "default"
	}

	if !h.policy.CheckRead(req.SubjectID) {
		h.policy.OnUnauthorized(req.SubjectID, "ENTRY.READ")
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("unauthorized"))
	}

	// Dispatch ENTRY.READ event
	ev := kernel.NewEvent("api", kernel.EvEntryRead, payload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := h.bus.Request(ctx, ev)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("request timeout"))
	}

	return websocket.WriteMessage(conn, ipc.MsgEntryResult, result.Payload)
}

// HandleUpdate processes an entry update request.
func (h *EntryHandler) HandleUpdate(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		SubjectID string            `json:"subject_id"`
		ID        string            `json:"id"`
		Title     string            `json:"title"`
		Fields    map[string]string `json:"fields"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}
	if req.SubjectID == "" {
		req.SubjectID = "default"
	}

	if !h.policy.CheckWrite(req.SubjectID) {
		h.policy.OnUnauthorized(req.SubjectID, "ENTRY.UPDATE")
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("unauthorized"))
	}

	// Dispatch ENTRY.UPDATE event
	ev := kernel.NewEvent("api", kernel.EvEntryUpdate, payload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := h.bus.Request(ctx, ev)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("request timeout"))
	}

	return websocket.WriteMessage(conn, ipc.MsgEntryResult, result.Payload)
}

// HandleDelete processes an entry deletion request.
func (h *EntryHandler) HandleDelete(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		SubjectID string `json:"subject_id"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("invalid request"))
	}
	if req.SubjectID == "" {
		req.SubjectID = "default"
	}

	if !h.policy.CheckWrite(req.SubjectID) {
		h.policy.OnUnauthorized(req.SubjectID, "ENTRY.DELETE")
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("unauthorized"))
	}

	// Dispatch ENTRY.DELETE event
	ev := kernel.NewEvent("api", kernel.EvEntryDelete, payload)
	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	result, err := h.bus.Request(ctx, ev)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("request timeout"))
	}

	return websocket.WriteMessage(conn, ipc.MsgEntryResult, result.Payload)
}

// HandleIngestBegin initiates a streaming file ingestion via io.Pipe.
func (h *EntryHandler) HandleIngestBegin(conn *gorillaws.Conn, payload []byte) error {
	var req struct {
		SubjectID string `json:"subject_id"`
		FileName  string `json:"file_name"`
		MIMEType  string `json:"mime_type"`
		TotalSize int64  `json:"total_size"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		log.Printf("[EntryHandler:IngestBegin] JSON unmarshal error: %v (payload=%q)", err, string(payload))
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("invalid request: "+err.Error()))
	}
	if req.SubjectID == "" {
		req.SubjectID = "default"
	}

	log.Printf("[EntryHandler:IngestBegin] subject_id=%q file=%q mime=%q size=%d",
		req.SubjectID, req.FileName, req.MIMEType, req.TotalSize)

	if !h.policy.CheckWrite(req.SubjectID) {
		log.Printf("[EntryHandler:IngestBegin] UNAUTHORIZED: subject_id=%q denied ENTRY.INGEST", req.SubjectID)
		h.policy.OnUnauthorized(req.SubjectID, "ENTRY.INGEST")
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("unauthorized: subject "+req.SubjectID+" denied"))
	}
	log.Printf("[EntryHandler:IngestBegin] policy OK for subject_id=%q", req.SubjectID)

	pr, pw := io.Pipe()

	// Create the activeIngest first, then register and launch the goroutine.
	// The goroutine receives the pointer directly to eliminate the map-lookup
	// race with HandleIngestEnd (which deletes the map entry before runIngest
	// might start for small/fast uploads).
	ai := &activeIngest{
		pw:       pw,
		manifest: make(chan storage.BlobManifest, 1),
		err:      make(chan error, 1),
	}

	h.mu.Lock()
	h.activeIngests[conn] = ai
	h.mu.Unlock()

	go h.runIngest(conn, pr, ingestReq{
		SubjectID: req.SubjectID,
		FileName:  req.FileName,
		MIMEType:  req.MIMEType,
		TotalSize: req.TotalSize,
	}, ai)

	ackPayload, _ := json.Marshal(map[string]string{"status": "ready"})
	return websocket.WriteMessage(conn, ipc.MsgAck, ackPayload)
}

// HandleChunk writes a chunk of file data to the active ingest pipe.
func (h *EntryHandler) HandleChunk(conn *gorillaws.Conn, payload []byte) error {
	h.mu.Lock()
	ingest, ok := h.activeIngests[conn]
	h.mu.Unlock()

	if !ok {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("no active ingest"))
	}

	// Write chunk to pipe (blocking if buffer full)
	_, err := ingest.pw.Write(payload)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("write failed"))
	}

	return nil
}

// HandleIngestEnd closes the ingest pipe and waits for completion.
// The IngestEngine.Ingest() method handles all flushing — this just waits for completion.
func (h *EntryHandler) HandleIngestEnd(conn *gorillaws.Conn) error {
	h.mu.Lock()
	ingest, ok := h.activeIngests[conn]
	if ok {
		delete(h.activeIngests, conn)
	}
	h.mu.Unlock()

	if !ok {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("no active ingest"))
	}

	// Close pipe to signal EOF to the ingest goroutine
	_ = ingest.pw.Close()

	// Wait for ingest completion (with timeout)
	select {
	case manifest := <-ingest.manifest:
		// Success: ingest completed and index was flushed
		result, _ := json.Marshal(manifest)
		return websocket.WriteMessage(conn, ipc.MsgEntryResult, result)
	case err := <-ingest.err:
		// Error during ingest
		log.Printf("[EntryHandler:IngestEnd] Ingest error: %v", err)
		return websocket.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	case <-time.After(5 * time.Minute):
		// Timeout waiting for ingest to complete
		log.Printf("[EntryHandler:IngestEnd] Ingest timeout after 5 minutes")
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("ingest timeout"))
	}
}

type ingestReq struct {
	SubjectID string
	FileName  string
	MIMEType  string
	TotalSize int64
}

// runIngest runs the ingest engine in a goroutine and reports progress.
// ingest is passed directly from HandleIngestBegin to avoid a map-lookup race
// with HandleIngestEnd, which deletes the map entry before this goroutine starts.
//
// CRITICAL: Progress writes MUST go through bridge.SafeWrite, NOT websocket.WriteMessage
// directly. gorilla/websocket is not goroutine-safe — a concurrent write from this
// goroutine while the readLoop holds connMu causes a panic + close 1005.
func (h *EntryHandler) runIngest(conn *gorillaws.Conn, r io.Reader, req ingestReq, ingest *activeIngest) {
	// Panic recovery: any panic in this goroutine must be caught here —
	// goroutine panics are not caught by the bridge readLoop recovery.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[EntryHandler:runIngest] PANIC recovered: %v — sending error to client", r)
			ingest.err <- fmt.Errorf("ingest panic: %v", r)
		}
	}()

	// Progress callback — MUST use bridge.SafeWrite (not websocket.WriteMessage directly)
	// to avoid concurrent-write panics on the gorilla websocket connection.
	progressFn := func(bytesRead, totalSize int64) {
		pct := 0.0
		if totalSize > 0 {
			pct = float64(bytesRead) / float64(totalSize)
		}
		progPayload, _ := json.Marshal(map[string]interface{}{
			"progress": pct,
			"stage":    "ingesting",
			"message":  fmt.Sprintf("%d / %d bytes", bytesRead, req.TotalSize),
		})
		if h.bridge != nil {
			// SafeWrite serializes with the readLoop via per-connection mutex
			_ = h.bridge.SafeWrite(conn, ipc.MsgIngestProgress, progPayload)
		}
		// If bridge is nil (e.g. in tests), progress updates are silently dropped
	}

	// Get MVK from locked memory
	mvk, ok := h.getMVK()
	if !ok || len(mvk) == 0 {
		log.Printf("[EntryHandler:runIngest] FAIL: MVK not available (handle=%q)", h.mvkHandle)
		ingest.err <- fmt.Errorf("vault locked: MVK not available — ensure vault is unlocked before uploading")
		return
	}
	log.Printf("[EntryHandler:runIngest] MVK retrieved OK — starting ingest file=%q mime=%q", req.FileName, req.MIMEType)

	// Run ingest
	ctx := context.Background()
	manifest, err := h.ingest.Ingest(ctx, mvk, req.FileName, req.MIMEType, r, progressFn)
	if err != nil {
		log.Printf("[EntryHandler:runIngest] Ingest error file=%q: %v", req.FileName, err)
		ingest.err <- err
		return
	}

	log.Printf("[EntryHandler:runIngest] Ingest complete file=%q manifestID=%s chunks=%d",
		req.FileName, manifest.ID, len(manifest.ChunkIDs))
	ingest.manifest <- manifest
}
