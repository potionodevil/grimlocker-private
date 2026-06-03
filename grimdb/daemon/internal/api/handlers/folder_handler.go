package handlers

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/daemon/internal/api/ipc"
	"github.com/grimlocker/grimdb/daemon/internal/ws"
	"github.com/grimlocker/grimdb/engine/storage"
)

// folderListItem is used in HandleFolderList responses.
type folderListItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Type     string `json:"type"` // "folder"
}

// fileListItem is used in HandleFolderList responses.
type fileListItem struct {
	ID              string `json:"id"`
	FileName        string `json:"file_name"`
	MIMEType        string `json:"mime_type"`
	TotalSize       int64  `json:"total_size"`
	ManifestBlockID string `json:"manifest_block_id"`
	FolderID        string `json:"folder_id"`
	Type            string `json:"type"` // "file"
}

// folderListResult is the full response payload for HandleFolderList.
type folderListResult struct {
	ParentID string           `json:"parent_id"`
	Folders  []folderListItem `json:"folders"`
	Files    []fileListItem   `json:"files"`
}

// HandleFolderCreate creates a new encrypted folder in the FileVault hierarchy.
func (h *EntryHandler) HandleFolderCreate(conn *gorillaws.Conn, payload []byte) error {
	if h.blockStore == nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("storage not available"))
	}
	var req struct {
		Name     string `json:"name"`
		ParentID string `json:"parent_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.Name == "" {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("name required"))
	}

	folder := storage.NewFolderEntry(req.Name, req.ParentID)
	folderJSON, _ := json.Marshal(folder)

	if err := h.blockStore.WriteBlock(storage.Block{
		ID:       folder.FolderBlockID(),
		Data:     folderJSON,
		Category: storage.CategoryFolder,
	}); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("create failed: "+err.Error()))
	}
	_ = h.blockStore.Flush()

	log.Printf("[FolderHandler:Create] folder=%q id=%s parent=%q", folder.Name, folder.ID, folder.ParentID)
	return h.writeFolderResult(conn, folder)
}

// HandleFolderList returns all folders and files in a parent folder (SKE-encrypted).
func (h *EntryHandler) HandleFolderList(conn *gorillaws.Conn, payload []byte, skeEncrypt func([]byte) (string, error)) error {
	if h.blockStore == nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("storage not available"))
	}
	var req struct {
		ParentID string `json:"parent_id"`
	}
	json.Unmarshal(payload, &req) //nolint:errcheck — defaults to root on failure

	metas, err := h.blockStore.ListBlocks()
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("list failed: "+err.Error()))
	}

	result := folderListResult{
		ParentID: req.ParentID,
		Folders:  make([]folderListItem, 0),
		Files:    make([]fileListItem, 0),
	}

	for _, meta := range metas {
		switch {
		case meta.Category == storage.CategoryFolder:
			block, err := h.blockStore.ReadBlock(meta.ID)
			if err != nil {
				continue
			}
			var folder storage.FolderEntry
			if json.Unmarshal(block.Data, &folder) != nil || folder.ID == "" {
				continue
			}
			if folder.ParentID == req.ParentID {
				result.Folders = append(result.Folders, folderListItem{
					ID: folder.ID, Name: folder.Name, ParentID: folder.ParentID, Type: "folder",
				})
			}

		case meta.Category == storage.CategoryFileVault && strings.HasSuffix(meta.ID, "-manifest"):
			block, err := h.blockStore.ReadBlock(meta.ID)
			if err != nil {
				continue
			}
			var manifest storage.BlobManifest
			if json.Unmarshal(block.Data, &manifest) != nil || manifest.ID == "" {
				continue
			}
			manifestBlockID := manifest.ManifestBlockID
			if manifestBlockID == "" {
				manifestBlockID = meta.ID
			}
			if manifest.FolderID == req.ParentID {
				result.Files = append(result.Files, fileListItem{
					ID:              manifest.ID,
					FileName:        manifest.FileName,
					MIMEType:        manifest.MIMEType,
					TotalSize:       manifest.TotalSize,
					ManifestBlockID: manifestBlockID,
					FolderID:        manifest.FolderID,
					Type:            "file",
				})
			}
		}
	}

	resultJSON, _ := json.Marshal(result)

	// SKE-encrypt for confidentiality in transit (same pattern as wsList).
	if skeEncrypt != nil {
		if encData, err := skeEncrypt(resultJSON); err == nil {
			resp, _ := json.Marshal(map[string]string{"encrypted": encData})
			return h.writeRaw(conn, ipc.MsgFolderResult, resp)
		}
	}
	return h.writeRaw(conn, ipc.MsgFolderResult, resultJSON)
}

// HandleFolderRename renames an existing folder.
func (h *EntryHandler) HandleFolderRename(conn *gorillaws.Conn, payload []byte) error {
	if h.blockStore == nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("storage not available"))
	}
	var req struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" || req.Name == "" {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("id and name required"))
	}

	blockID := "folder-" + req.ID
	block, err := h.blockStore.ReadBlock(blockID)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("folder not found"))
	}
	var folder storage.FolderEntry
	if err := json.Unmarshal(block.Data, &folder); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("corrupt folder data"))
	}

	folder.Name = req.Name
	folder.UpdatedAt = time.Now().UnixNano()

	updated, _ := json.Marshal(folder)
	_ = h.blockStore.WriteBlock(storage.Block{ID: blockID, Data: updated, Category: storage.CategoryFolder})
	_ = h.blockStore.Flush()
	return h.writeFolderResult(conn, folder)
}

// HandleFolderDelete deletes a folder, moving its files to the parent.
func (h *EntryHandler) HandleFolderDelete(conn *gorillaws.Conn, payload []byte) error {
	if h.blockStore == nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("storage not available"))
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.ID == "" {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("id required"))
	}

	blockID := "folder-" + req.ID
	block, err := h.blockStore.ReadBlock(blockID)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("folder not found"))
	}
	var folder storage.FolderEntry
	json.Unmarshal(block.Data, &folder) //nolint:errcheck

	metas, _ := h.blockStore.ListBlocks()

	// Move files to parent folder.
	for _, meta := range metas {
		if meta.Category != storage.CategoryFileVault || !strings.HasSuffix(meta.ID, "-manifest") {
			continue
		}
		b, err := h.blockStore.ReadBlock(meta.ID)
		if err != nil {
			continue
		}
		var manifest storage.BlobManifest
		if json.Unmarshal(b.Data, &manifest) != nil || manifest.FolderID != req.ID {
			continue
		}
		manifest.FolderID = folder.ParentID
		if updated, err := json.Marshal(manifest); err == nil {
			_ = h.blockStore.WriteBlock(storage.Block{ID: meta.ID, Data: updated, Category: storage.CategoryFileVault})
		}
	}

	// Delete direct subfolders (one level only — shallow delete for MVP).
	for _, meta := range metas {
		if meta.Category != storage.CategoryFolder {
			continue
		}
		b, err := h.blockStore.ReadBlock(meta.ID)
		if err != nil {
			continue
		}
		var sub storage.FolderEntry
		if json.Unmarshal(b.Data, &sub) != nil || sub.ParentID != req.ID {
			continue
		}
		_ = h.blockStore.DeleteBlock(meta.ID)
	}

	if err := h.blockStore.DeleteBlock(blockID); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("delete failed: "+err.Error()))
	}
	_ = h.blockStore.Flush()

	log.Printf("[FolderHandler:Delete] id=%s files→parent=%q", req.ID, folder.ParentID)
	result, _ := json.Marshal(map[string]string{"deleted": req.ID})
	return h.writeRaw(conn, ipc.MsgFolderResult, result)
}

// HandleFileMoveToFolder moves a file manifest to a different folder.
func (h *EntryHandler) HandleFileMoveToFolder(conn *gorillaws.Conn, payload []byte) error {
	if h.blockStore == nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("storage not available"))
	}
	var req struct {
		ManifestBlockID string `json:"manifest_block_id"`
		FolderID        string `json:"folder_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil || req.ManifestBlockID == "" {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("manifest_block_id required"))
	}

	blockID := req.ManifestBlockID
	if !strings.HasPrefix(blockID, "blob-") {
		blockID = "blob-" + blockID + "-manifest"
	}

	block, err := h.blockStore.ReadBlock(blockID)
	if err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("file not found"))
	}
	var manifest storage.BlobManifest
	if err := json.Unmarshal(block.Data, &manifest); err != nil {
		return websocket.WriteMessage(conn, ipc.MsgError, []byte("corrupt manifest"))
	}

	manifest.FolderID = req.FolderID
	updated, _ := json.Marshal(manifest)
	_ = h.blockStore.WriteBlock(storage.Block{ID: blockID, Data: updated, Category: storage.CategoryFileVault})
	_ = h.blockStore.Flush()

	result, _ := json.Marshal(map[string]string{"moved": req.ManifestBlockID, "folder_id": req.FolderID})
	return h.writeRaw(conn, ipc.MsgFolderResult, result)
}

// writeFolderResult serialises v and sends it as MsgFolderResult.
func (h *EntryHandler) writeFolderResult(conn *gorillaws.Conn, v interface{}) error {
	data, _ := json.Marshal(v)
	return h.writeRaw(conn, ipc.MsgFolderResult, data)
}

// writeRaw sends a raw byte payload using a direct write (NOT SafeWrite).
//
// Folder handlers are called synchronously from the bridge readLoop, which
// already holds connMu. Calling SafeWrite here would try to re-acquire connMu
// and deadlock. Direct websocket.WriteMessage is safe because the readLoop is
// the only goroutine writing at this point (connMu is held as the exclusive lock).
func (h *EntryHandler) writeRaw(conn *gorillaws.Conn, msgType byte, data []byte) error {
	return websocket.WriteMessage(conn, msgType, data)
}
