// Package api provides the JSON-over-HTTP entry point for SDK clients and tooling.
// This is distinct from the binary WebSocket protocol in translator.go, which serves
// the Tauri frontend. Channel-restriction is enforced as middleware: requests targeting
// SECURITY or CRYPTO channels are rejected with 403 Forbidden.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/daemon/internal/workspace"
)

// actionMap translates JSON action names to kernel EventType values.
// Only actions in this table are accepted; all others return 400.
var actionMap = map[string]kernel.EventType{
	// Auth / session
	"vault.unlock": kernel.EvAuthUnlock,
	"vault.init":   kernel.EvAuthSetup,
	"vault.status": kernel.EvAuthStatus,
	"vault.logout": kernel.EvAuthLogout,

	// Raw block storage (low-level)
	"storage.write":  kernel.EvStorageWrite,
	"storage.read":   kernel.EvStorageRead,
	"storage.delete": kernel.EvStorageDelete,
	"storage.list":   kernel.EvStorageList,

	// High-level vault entries (CLI and REST clients)
	"entry.create":  kernel.EvEntryCreate,
	"entry.read":    kernel.EvEntryRead,
	"entry.update":  kernel.EvEntryUpdate,
	"entry.delete":  kernel.EvEntryDelete,
	"entry.query":   kernel.EvEntryQuery,
	"entry.history": kernel.EvEntryHistory,
	"entry.restore": kernel.EvEntryRestore,

	// Tool operations
	"tool.ssh_gen":    kernel.EvToolSSHGen,
	"totp.generate":   kernel.EvTOTPGenerate,
	"health.analyze":  kernel.EvHealthAnalyze,
	"import.csv":      kernel.EvImportCSV,
	"shamir.split":    kernel.EvShamirSplit,
	"shamir.combine":  kernel.EvShamirCombine,
	"share.create":    kernel.EvShareCreate,
	"share.redeem":    kernel.EvShareRedeem,
	"share.revoke":    kernel.EvShareRevoke,

	// Sync
	"sync.begin": kernel.EvSyncBegin,
}

// blockedChannels are channels that the UI and SDK may never dispatch to directly.
// Requests resolving to these channels are rejected with 403 Forbidden.
var blockedChannels = map[string]bool{
	"SECURITY": true,
	"CRYPTO":   true,
}

// Handler is the JSON IPC entry point. Mount on /api/v1 of the IPC mux.
type Handler struct {
	bus          kernel.Dispatcher
	workspaceMgr *workspace.WorkspaceManager
	blockStore   storage.BlockStore
	ingestEngine *storage.IngestEngine
	mvkResolver  func() []byte
	auditLog     security.AuditLog
	syncStatusFn func() ([]byte, error)
	syncTrigFn   func()
}

// NewHandler creates a Handler backed by the given dispatcher.
func NewHandler(bus kernel.Dispatcher) *Handler {
	return &Handler{bus: bus}
}

// SetWorkspaceManager injects the WorkspaceManager for workspace.* actions.
func (h *Handler) SetWorkspaceManager(wm *workspace.WorkspaceManager) {
	h.workspaceMgr = wm
}

// SetBlockStore injects the BlockStore for folder/file actions.
func (h *Handler) SetBlockStore(bs storage.BlockStore) {
	h.blockStore = bs
}

// SetIngestEngine injects the IngestEngine for file upload/download.
func (h *Handler) SetIngestEngine(ie *storage.IngestEngine) {
	h.ingestEngine = ie
}

// SetMVKResolver injects the MVK resolver for file operations.
func (h *Handler) SetMVKResolver(fn func() []byte) {
	h.mvkResolver = fn
}

// SetAuditLog injects the AuditLog for audit.list.
func (h *Handler) SetAuditLog(al security.AuditLog) {
	h.auditLog = al
}

// SetSyncFns injects sync callbacks for sync.status and sync.trigger.
func (h *Handler) SetSyncFns(statusFn func() ([]byte, error), triggerFn func()) {
	h.syncStatusFn = statusFn
	h.syncTrigFn = triggerFn
}

// ipcRequest is the JSON body accepted by ServeHTTP.
type ipcRequest struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ipcResponse is the JSON body written by ServeHTTP.
type ipcResponse struct {
	OK        bool            `json:"ok"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
	ErrorCode int             `json:"error_code,omitempty"` // GrimlockError code for SDK clients
}

// ServeHTTP reads {"action":"...","payload":{...}}, validates channel restrictions,
// dispatches to the bus, and writes the result as JSON. It enforces a 30s timeout
// per request to match the WebSocket translator behaviour.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "method not allowed"})
		return
	}

	var req ipcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "invalid JSON body"})
		return
	}

	// Inline handlers for operations not routed through the bus.
	switch {
	case strings.HasPrefix(req.Action, "workspace."):
		h.handleWorkspace(w, req)
		return
	case strings.HasPrefix(req.Action, "sync.") && req.Action != "sync.begin":
		h.handleSyncInline(w, req)
		return
	case req.Action == "audit.list":
		h.handleAudit(w, req)
		return
	case strings.HasPrefix(req.Action, "folder."):
		h.handleFolder(w, req)
		return
	case req.Action == "file.upload":
		h.handleFileUpload(w, req)
		return
	case req.Action == "file.download":
		h.handleFileDownload(w, req)
		return
	case req.Action == "file.move":
		h.handleFileMove(w, req)
		return
	}

	evType, ok := actionMap[req.Action]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "unknown action"})
		return
	}

	if blockedChannels[evType.Channel()] {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "channel restricted"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	payload := []byte(req.Payload)
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	ev := kernel.NewEvent("ipc-handler", evType, payload)
	reply, err := h.bus.Request(ctx, ev)
	if err != nil {
		// If the error is a typed GrimlockError, use its HTTP status and code.
		status := http.StatusInternalServerError
		code := 0
		if ge, ok := err.(*gerrors.GrimlockError); ok {
			status = ge.HTTPStatus()
			code = ge.Code
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error(), ErrorCode: code})
		return
	}

	_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: reply.Payload})
}

// ── Workspace inline handlers ────────────────────────────────────────────────

func (h *Handler) handleWorkspace(w http.ResponseWriter, req ipcRequest) {
	if h.workspaceMgr == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "workspace manager unavailable"})
		return
	}

	var err error
	var payload interface{}

	switch req.Action {
	case "workspace.list":
		payload = h.workspaceMgr.List()

	case "workspace.create":
		var r struct{ Name string }
		if json.Unmarshal(req.Payload, &r) != nil || r.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "name required"})
			return
		}
		ws, e := h.workspaceMgr.Create(r.Name)
		if e != nil {
			err = e
		} else {
			payload = ws
		}

	case "workspace.switch":
		var r struct{ ID string }
		if json.Unmarshal(req.Payload, &r) != nil || r.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "id required"})
			return
		}
		ws, e := h.workspaceMgr.Switch(r.ID)
		if e != nil {
			err = e
		} else {
			payload = ws
		}

	case "workspace.rename":
		var r struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(req.Payload, &r) != nil || r.ID == "" || r.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "id and name required"})
			return
		}
		if e := h.workspaceMgr.Rename(r.ID, r.Name); e != nil {
			err = e
		} else {
			payload = h.workspaceMgr.List()
		}

	case "workspace.delete":
		var r struct{ ID string }
		if json.Unmarshal(req.Payload, &r) != nil || r.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "id required"})
			return
		}
		err = h.workspaceMgr.Delete(r.ID)

	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "unknown workspace action"})
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error()})
		return
	}

	if payload != nil {
		b, _ := json.Marshal(payload)
		_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: b})
	} else {
		ack, _ := json.Marshal(map[string]string{"status": "ok"})
		_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: ack})
	}
}

// ── Sync inline handlers ─────────────────────────────────────────────────────

func (h *Handler) handleSyncInline(w http.ResponseWriter, req ipcRequest) {
	switch req.Action {
	case "sync.status":
		if h.syncStatusFn == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "sync unavailable"})
			return
		}
		data, err := h.syncStatusFn()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: data})

	case "sync.trigger":
		if h.syncTrigFn == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "sync unavailable"})
			return
		}
		h.syncTrigFn()
		ack, _ := json.Marshal(map[string]string{"ok": "true"})
		_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: ack})

	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "unknown sync action"})
	}
}

// ── Audit inline handler ─────────────────────────────────────────────────────

func (h *Handler) handleAudit(w http.ResponseWriter, req ipcRequest) {
	if h.auditLog == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "audit unavailable"})
		return
	}
	var r struct{ Count int }
	n := 50
	if json.Unmarshal(req.Payload, &r) == nil && r.Count > 0 && r.Count <= 500 {
		n = r.Count
	}
	events := h.auditLog.Recent(n)
	b, _ := json.Marshal(events)
	_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: b})
}

// ── Folder inline handlers ───────────────────────────────────────────────────

func (h *Handler) handleFolder(w http.ResponseWriter, req ipcRequest) {
	if h.blockStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "storage not available"})
		return
	}

	var err error
	var payload interface{}

	switch req.Action {
	case "folder.create":
		var r struct {
			Name     string `json:"name"`
			ParentID string `json:"parent_id"`
		}
		if json.Unmarshal(req.Payload, &r) != nil || r.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "name required"})
			return
		}
		folder := storage.NewFolderEntry(r.Name, r.ParentID)
		folderJSON, _ := json.Marshal(folder)
		if e := h.blockStore.WriteBlock(storage.Block{
			ID:       folder.FolderBlockID(),
			Data:     folderJSON,
			Category: storage.CategoryFolder,
		}); e != nil {
			err = fmt.Errorf("create folder: %w", e)
		} else {
			_ = h.blockStore.Flush()
			payload = folder
		}

	case "folder.list":
		var r struct{ ParentID string }
		json.Unmarshal(req.Payload, &r)

		metas, e := h.blockStore.ListBlocks()
		if e != nil {
			err = fmt.Errorf("list blocks: %w", e)
			break
		}
		type folderItem struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			ParentID string `json:"parent_id"`
			Type     string `json:"type"`
		}
		type fileItem struct {
			ID              string `json:"id"`
			FileName        string `json:"file_name"`
			MIMEType        string `json:"mime_type"`
			TotalSize       int64  `json:"total_size"`
			ManifestBlockID string `json:"manifest_block_id"`
			FolderID        string `json:"folder_id"`
			Type            string `json:"type"`
		}
		result := struct {
			ParentID string       `json:"parent_id"`
			Folders  []folderItem `json:"folders"`
			Files    []fileItem   `json:"files"`
		}{ParentID: r.ParentID, Folders: []folderItem{}, Files: []fileItem{}}

		for _, meta := range metas {
			switch {
			case meta.Category == storage.CategoryFolder:
				block, e2 := h.blockStore.ReadBlock(meta.ID)
				if e2 != nil {
					continue
				}
				var f storage.FolderEntry
				if json.Unmarshal(block.Data, &f) != nil || f.ID == "" {
					continue
				}
				if f.ParentID == r.ParentID {
					result.Folders = append(result.Folders, folderItem{
						ID: f.ID, Name: f.Name, ParentID: f.ParentID, Type: "folder",
					})
				}
			case meta.Category == storage.CategoryFileVault && strings.HasSuffix(meta.ID, "-manifest"):
				block, e2 := h.blockStore.ReadBlock(meta.ID)
				if e2 != nil {
					continue
				}
				var manifest storage.BlobManifest
				if json.Unmarshal(block.Data, &manifest) != nil || manifest.ID == "" {
					continue
				}
				mbID := manifest.ManifestBlockID
				if mbID == "" {
					mbID = meta.ID
				}
				if manifest.FolderID == r.ParentID {
					result.Files = append(result.Files, fileItem{
						ID:              manifest.ID,
						FileName:        manifest.FileName,
						MIMEType:        manifest.MIMEType,
						TotalSize:       manifest.TotalSize,
						ManifestBlockID: mbID,
						FolderID:        manifest.FolderID,
						Type:            "file",
					})
				}
			}
		}
		b, _ := json.Marshal(result)
		payload = json.RawMessage(b)

	case "folder.rename":
		var r struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(req.Payload, &r) != nil || r.ID == "" || r.Name == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "id and name required"})
			return
		}
		blockID := "folder-" + r.ID
		block, e := h.blockStore.ReadBlock(blockID)
		if e != nil {
			err = fmt.Errorf("folder not found")
			break
		}
		var folder storage.FolderEntry
		if json.Unmarshal(block.Data, &folder) != nil {
			err = fmt.Errorf("corrupt folder data")
			break
		}
		folder.Name = r.Name
		folder.UpdatedAt = time.Now().UnixNano()
		updated, _ := json.Marshal(folder)
		_ = h.blockStore.WriteBlock(storage.Block{ID: blockID, Data: updated, Category: storage.CategoryFolder})
		_ = h.blockStore.Flush()
		payload = folder

	case "folder.delete":
		var r struct{ ID string }
		if json.Unmarshal(req.Payload, &r) != nil || r.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(ipcResponse{Error: "id required"})
			return
		}
		blockID := "folder-" + r.ID
		block, e := h.blockStore.ReadBlock(blockID)
		if e != nil {
			err = fmt.Errorf("folder not found")
			break
		}
		var folder storage.FolderEntry
		json.Unmarshal(block.Data, &folder)

		metas, _ := h.blockStore.ListBlocks()
		// Move files to parent
		for _, meta := range metas {
			if meta.Category != storage.CategoryFileVault || !strings.HasSuffix(meta.ID, "-manifest") {
				continue
			}
			b, e2 := h.blockStore.ReadBlock(meta.ID)
			if e2 != nil {
				continue
			}
			var mf storage.BlobManifest
			if json.Unmarshal(b.Data, &mf) != nil || mf.FolderID != r.ID {
				continue
			}
			mf.FolderID = folder.ParentID
			if updated, e3 := json.Marshal(mf); e3 == nil {
				_ = h.blockStore.WriteBlock(storage.Block{ID: meta.ID, Data: updated, Category: storage.CategoryFileVault})
			}
		}
		// Delete subfolders
		for _, meta := range metas {
			if meta.Category != storage.CategoryFolder {
				continue
			}
			b, e2 := h.blockStore.ReadBlock(meta.ID)
			if e2 != nil {
				continue
			}
			var sub storage.FolderEntry
			if json.Unmarshal(b.Data, &sub) != nil || sub.ParentID != r.ID {
				continue
			}
			_ = h.blockStore.DeleteBlock(meta.ID)
		}
		if e := h.blockStore.DeleteBlock(blockID); e != nil {
			err = fmt.Errorf("delete folder: %w", e)
		} else {
			_ = h.blockStore.Flush()
			payload = map[string]string{"deleted": r.ID}
		}

	default:
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "unknown folder action"})
		return
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error()})
		return
	}
	if payload != nil {
		b, _ := json.Marshal(payload)
		_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: b})
	}
}

// ── File upload / download inline handlers ───────────────────────────────────

func (h *Handler) handleFileUpload(w http.ResponseWriter, req ipcRequest) {
	if h.ingestEngine == nil || h.blockStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "ingest engine unavailable"})
		return
	}
	mvk := h.mvkResolver()
	if mvk == nil {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "vault locked"})
		return
	}

	var r struct {
		FileName  string `json:"file_name"`
		MimeType  string `json:"mime_type"`
		DataB64   string `json:"data_b64"`
		TotalSize int    `json:"total_size"`
		FolderID  string `json:"folder_id"`
	}
	if err := json.Unmarshal(req.Payload, &r); err != nil || r.FileName == "" || r.DataB64 == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "file_name and data_b64 required"})
		return
	}

	decoded, err := base64.StdEncoding.DecodeString(r.DataB64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "invalid base64 data"})
		return
	}

	const maxSize = 100 * 1024 * 1024
	if len(decoded) > maxSize {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: fmt.Sprintf("file too large: %d bytes (max %d)", len(decoded), maxSize)})
		return
	}

	manifest, err := h.ingestEngine.Ingest(
		context.Background(),
		mvk,
		r.FileName,
		r.MimeType,
		bytes.NewReader(decoded),
		nil,
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error()})
		return
	}

	// Tag the manifest with folder
	if r.FolderID != "" {
		manifest.FolderID = r.FolderID
		if manifestJSON, e := json.Marshal(manifest); e == nil {
			_ = h.blockStore.WriteBlock(storage.Block{
				ID: manifest.ManifestBlockID, Data: manifestJSON, Category: storage.CategoryFileVault,
			})
			_ = h.blockStore.Flush()
		}
	}

	respPayload, _ := json.Marshal(map[string]interface{}{
		"manifest_block_id": manifest.ManifestBlockID,
		"id":                manifest.ID,
		"file_name":         manifest.FileName,
		"mime_type":         manifest.MIMEType,
		"total_size":        manifest.TotalSize,
		"sha256":            fmt.Sprintf("%x", manifest.SHA256),
		"chunks":            len(manifest.ChunkIDs),
	})
	_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: respPayload})
}

func (h *Handler) handleFileDownload(w http.ResponseWriter, req ipcRequest) {
	if h.ingestEngine == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "ingest engine unavailable"})
		return
	}
	mvk := h.mvkResolver()
	if mvk == nil {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "vault locked"})
		return
	}

	var r struct {
		ManifestBlockID string `json:"manifest_block_id"`
	}
	if err := json.Unmarshal(req.Payload, &r); err != nil || r.ManifestBlockID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "manifest_block_id required"})
		return
	}

	manifestBlockID := r.ManifestBlockID
	if !strings.HasPrefix(manifestBlockID, "blob-") {
		manifestBlockID = "blob-" + manifestBlockID + "-manifest"
	}

	manifest, err := h.ingestEngine.ReadManifest(manifestBlockID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error()})
		return
	}

	var buf bytes.Buffer
	if err := h.ingestEngine.RetrieveBlob(context.Background(), mvk, manifest, &buf); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error()})
		return
	}

	respPayload, _ := json.Marshal(map[string]interface{}{
		"file_name":         manifest.FileName,
		"mime_type":         manifest.MIMEType,
		"data_b64":          base64.StdEncoding.EncodeToString(buf.Bytes()),
		"total_size":        manifest.TotalSize,
		"sha256":            fmt.Sprintf("%x", manifest.SHA256),
		"manifest_block_id": manifest.ManifestBlockID,
	})
	_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: respPayload})
}

// ── File move inline handler ─────────────────────────────────────────────────

func (h *Handler) handleFileMove(w http.ResponseWriter, req ipcRequest) {
	if h.blockStore == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "storage not available"})
		return
	}

	var r struct {
		ManifestBlockID string `json:"manifest_block_id"`
		FolderID        string `json:"folder_id"`
	}
	if err := json.Unmarshal(req.Payload, &r); err != nil || r.ManifestBlockID == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "manifest_block_id required"})
		return
	}

	blockID := r.ManifestBlockID
	if !strings.HasPrefix(blockID, "blob-") {
		blockID = "blob-" + blockID + "-manifest"
	}

	block, err := h.blockStore.ReadBlock(blockID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "file not found"})
		return
	}
	var manifest storage.BlobManifest
	if err := json.Unmarshal(block.Data, &manifest); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "corrupt manifest"})
		return
	}

	manifest.FolderID = r.FolderID
	updated, _ := json.Marshal(manifest)
	_ = h.blockStore.WriteBlock(storage.Block{ID: blockID, Data: updated, Category: storage.CategoryFileVault})
	_ = h.blockStore.Flush()

	respPayload, _ := json.Marshal(map[string]string{"moved": r.ManifestBlockID, "folder_id": r.FolderID})
	_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: respPayload})
}

// Ensure unused import compilation guard.
var _ = io.Discard
