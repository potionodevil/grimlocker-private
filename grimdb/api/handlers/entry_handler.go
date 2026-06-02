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
	gerrors "github.com/grimlocker/grimdb/errors"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/storage"
)

const requestTimeout = 30 * time.Second

type activeIngest struct {
	pw        *io.PipeWriter
	manifest  chan storage.BlobManifest
	err       chan error
	subjectID string // retained for per-chunk auth re-verification
}

// EntryHandler manages CRUD operations and file streaming for vault entries.
type EntryHandler struct {
	bus           kernel.Dispatcher
	bridge        *websocket.Bridge
	policy        *PolicyManager
	ingest        *storage.IngestEngine
	blockStore    storage.BlockStore               // direct store access for folder CRUD
	retrieveMVK   func(string) ([]byte, bool)      // security.Module.RetrieveMVK
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

// SetBlockStore wires the direct block store for folder CRUD operations.
func (h *EntryHandler) SetBlockStore(bs storage.BlockStore) {
	h.blockStore = bs
}

// SetMVKHandle sets the current MVK handle (called after successful AUTH.UNLOCK).
func (h *EntryHandler) SetMVKHandle(handle string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mvkHandle = handle
}

// getMVK retrieves the current MVK from the handle.
// Holds the mutex through the full retrieveMVK call to prevent a TOCTOU race
// where the session could be locked between the handle check and the retrieval.
func (h *EntryHandler) getMVK() ([]byte, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.mvkHandle == "" {
		return nil, false
	}
	return h.retrieveMVK(h.mvkHandle)
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
		log.Printf("[EntryHandler:AUTH.RESULT] success=%v ReplyTo=%s",
			res.Success, e.ReplyTo)
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
		FolderID  string `json:"folder_id"` // optional: which folder to place the file in
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

	const maxFileSize int64 = 100 * 1024 * 1024 // 100 MB
	if req.TotalSize > maxFileSize {
		log.Printf("[EntryHandler:IngestBegin] REJECTED: file too large size=%d (max=%d)", req.TotalSize, maxFileSize)
		return websocket.WriteMessage(conn, ipc.MsgError, []byte(
			fmt.Sprintf("file too large: %d bytes (max 100 MB)", req.TotalSize)))
	}

	pr, pw := io.Pipe()

	// Create the activeIngest first, then register and launch the goroutine.
	// The goroutine receives the pointer directly to eliminate the map-lookup
	// race with HandleIngestEnd (which deletes the map entry before runIngest
	// might start for small/fast uploads).
	ai := &activeIngest{
		pw:        pw,
		manifest:  make(chan storage.BlobManifest, 1),
		err:       make(chan error, 1),
		subjectID: req.SubjectID,
	}

	h.mu.Lock()
	h.activeIngests[conn] = ai
	h.mu.Unlock()

	go h.runIngest(conn, pr, ingestReq{
		SubjectID: req.SubjectID,
		FileName:  req.FileName,
		MIMEType:  req.MIMEType,
		TotalSize: req.TotalSize,
		FolderID:  req.FolderID,
	}, ai)

	ackPayload, _ := json.Marshal(map[string]string{"status": "ready"})
	return websocket.WriteMessage(conn, ipc.MsgAck, ackPayload)
}

// HandleChunk writes a chunk of file data to the active ingest pipe.
// Re-verifies subject_id on every chunk to prevent data injection if the
// session is locked or permissions are revoked mid-upload.
func (h *EntryHandler) HandleChunk(conn *gorillaws.Conn, payload []byte) error {
	h.mu.Lock()
	ingest, ok := h.activeIngests[conn]
	h.mu.Unlock()

	if !ok {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("no active ingest"))
	}

	// Re-verify subject_id on each chunk. If the session was locked after
	// HandleIngestBegin completed, we must stop accepting data.
	if !h.policy.CheckWrite(ingest.subjectID) {
		log.Printf("[EntryHandler:HandleChunk] UNAUTHORIZED mid-upload: subject_id=%q — aborting ingest",
			ingest.subjectID)
		h.policy.OnUnauthorized(ingest.subjectID, "ENTRY.INGEST_CHUNK")
		_ = ingest.pw.CloseWithError(fmt.Errorf("session revoked mid-upload"))
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("unauthorized: session revoked"))
	}

	// Write chunk to pipe (blocking if buffer full)
	if _, err := ingest.pw.Write(payload); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("write failed"))
	}

	return nil
}

// HandleIngestEnd closes the ingest pipe and waits for completion asynchronously.
// This MUST NOT block synchronously because the bridge readLoop holds connMu during
// the handler call — blocking here while runIngest tries to SafeWrite(connMu) would deadlock.
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

	// Async wait — readLoop holds connMu, so we must not block here.
	go func() {
		select {
		case manifest := <-ingest.manifest:
			result, _ := json.Marshal(manifest)
			if h.bridge != nil {
				_ = h.bridge.SafeWrite(conn, ipc.MsgEntryResult, result)
			}
		case err := <-ingest.err:
			// Convert to structured JSON with error code if it's a GrimlockError.
			if ge, ok := err.(*gerrors.GrimlockError); ok {
				log.Printf("[EntryHandler:IngestEnd] [Code %d] Ingest error: %s — op=%s blockID=%s",
					ge.Code, ge.Message, ge.Ctx.Operation, ge.Ctx.BlockID)
				if h.bridge != nil {
					errPayload, _ := ge.MarshalJSON()
					_ = h.bridge.SafeWrite(conn, ipc.MsgError, errPayload)
				}
			} else {
				log.Printf("[EntryHandler:IngestEnd] Ingest error: %v", err)
				if h.bridge != nil {
					errPayload, _ := json.Marshal(map[string]interface{}{
						"error":      err.Error(),
						"error_code": 0,
					})
					_ = h.bridge.SafeWrite(conn, ipc.MsgError, errPayload)
				}
			}
		case <-time.After(5 * time.Minute):
			log.Printf("[EntryHandler:IngestEnd] Ingest timeout after 5 minutes")
			if h.bridge != nil {
				te := gerrors.NewBusTimeoutError("FILE_INGEST")
				errPayload, _ := te.MarshalJSON()
				_ = h.bridge.SafeWrite(conn, ipc.MsgError, errPayload)
			}
		}
	}()

	return nil
}

// HandleFileDownload decrypts and streams a stored file back to the client.
// It reads the BlobManifest by manifest_block_id, then calls IngestEngine.RetrieveBlob
// to decrypt all chunks, streaming each as MsgFileChunkData binary frames.
// The download is performed in a background goroutine to avoid blocking the readLoop.
func (h *EntryHandler) HandleFileDownload(
	conn *gorillaws.Conn,
	manifestBlockID string,
	mvk []byte,
	bridge *websocket.Bridge,
) error {
	if h.ingest == nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("ingest engine unavailable"))
	}
	if len(mvk) == 0 {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("vault locked"))
	}

	// Read and parse the manifest block.
	manifest, err := h.ingest.ReadManifest(manifestBlockID)
	if err != nil {
		if ge, ok := err.(*gerrors.GrimlockError); ok {
			errPayload, _ := ge.MarshalJSON()
			return websocket.WriteMessage(conn, ipc.MsgError, errPayload)
		}
		return websocket.WriteMessage(conn, ipc.MsgError, []byte(err.Error()))
	}

	// Stream download in background goroutine — bridge.SafeWrite prevents
	// concurrent write races with the readLoop.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[EntryHandler:FileDownload] PANIC: %v", r)
				if bridge != nil {
					_ = bridge.SafeWrite(conn, ipc.MsgError, []byte("download panic"))
				}
			}
		}()

		ctx := context.Background()
		pr, pw := io.Pipe()

		// Producer: RetrieveBlob writes decrypted data into the pipe.
		go func() {
			err := h.ingest.RetrieveBlob(ctx, mvk, manifest, pw)
			if err != nil {
				pw.CloseWithError(err)
			} else {
				pw.Close()
			}
		}()

		// Consumer: read chunks and send as MsgFileChunkData.
		const chunkSize = 64 * 1024 // 64KB per WebSocket frame
		buf := make([]byte, chunkSize)
		for {
			n, readErr := pr.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				if bridge != nil {
					if writeErr := bridge.SafeWrite(conn, ipc.MsgFileChunkData, chunk); writeErr != nil {
						log.Printf("[EntryHandler:FileDownload] write error: %v", writeErr)
						return
					}
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					break
				}
				log.Printf("[EntryHandler:FileDownload] read error: %v", readErr)
				if bridge != nil {
					if ge, ok := readErr.(*gerrors.GrimlockError); ok {
						errPayload, _ := ge.MarshalJSON()
						_ = bridge.SafeWrite(conn, ipc.MsgError, errPayload)
					} else {
						_ = bridge.SafeWrite(conn, ipc.MsgError, []byte(readErr.Error()))
					}
				}
				return
			}
		}

		// Send completion frame with manifest metadata.
		endPayload, _ := json.Marshal(map[string]interface{}{
			"sha256":     fmt.Sprintf("%x", manifest.SHA256),
			"total_size": manifest.TotalSize,
			"file_name":  manifest.FileName,
			"mime_type":  manifest.MIMEType,
		})
		if bridge != nil {
			_ = bridge.SafeWrite(conn, ipc.MsgFileDownloadEnd, endPayload)
		}
		log.Printf("[EntryHandler:FileDownload] Complete file=%q size=%d", manifest.FileName, manifest.TotalSize)
	}()

	return nil
}

type ingestReq struct {
	SubjectID string
	FileName  string
	MIMEType  string
	TotalSize int64
	FolderID  string // optional: target folder in FileVault hierarchy
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

	// Progress callback — fires a separate goroutine for each update to avoid deadlock.
	//
	// Deadlock scenario without the goroutine:
	//   readLoop holds connMu → blocked in HandleChunk.pw.Write() waiting for ingest to Read.
	//   ingest goroutine calls progressFn → SafeWrite → waits for connMu → DEADLOCK.
	//
	// By launching SafeWrite in a goroutine, progressFn returns immediately and the ingest
	// goroutine can call r.Read(), unblocking the readLoop. Progress updates are best-effort
	// (dropped on connection close), which is acceptable for informational feedback.
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
			go func() {
				_ = h.bridge.SafeWrite(conn, ipc.MsgIngestProgress, progPayload)
			}()
		}
	}

	// Get MVK from locked memory
	mvk, ok := h.getMVK()
	if !ok || len(mvk) == 0 {
		log.Printf("[EntryHandler:runIngest] FAIL: MVK not available (handle=<redacted>)")
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

	// Tag the manifest with the folder it belongs to (if any).
	if req.FolderID != "" {
		manifest.FolderID = req.FolderID
		// Re-write the manifest block with the updated FolderID.
		if h.blockStore != nil {
			if manifestJSON, mErr := json.Marshal(manifest); mErr == nil {
				_ = h.blockStore.WriteBlock(storage.Block{
					ID:       manifest.ManifestBlockID,
					Data:     manifestJSON,
					Category: storage.CategoryFileVault,
				})
				_ = h.blockStore.Flush()
			}
		}
	}

	log.Printf("[EntryHandler:runIngest] Ingest complete file=%q manifestID=%s chunks=%d folder=%q",
		req.FileName, manifest.ID, len(manifest.ChunkIDs), req.FolderID)
	ingest.manifest <- manifest
}

// CleanupConn aborts any active ingest for the given connection.
// Called on WebSocket disconnect to rollback orphaned chunk blocks.
func (h *EntryHandler) CleanupConn(conn *gorillaws.Conn) {
	h.mu.Lock()
	ingest, ok := h.activeIngests[conn]
	if ok {
		delete(h.activeIngests, conn)
	}
	h.mu.Unlock()

	if ok {
		// CloseWithError causes runIngest's r.Read() to fail →
		// deleteChunks() is called automatically inside IngestEngine (rollback).
		_ = ingest.pw.CloseWithError(fmt.Errorf("connection closed"))
		log.Printf("[EntryHandler:CleanupConn] Aborted active ingest for %s (rollback triggered)", conn.RemoteAddr())
	}
}
