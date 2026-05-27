package grimdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
)

const adapterModuleID = "storage"

// storagePayload is the JSON schema for STORAGE.WRITE events.
type storagePayload struct {
	Block storage.Block `json:"block"`
}

// storageResult is the JSON schema for STORAGE.RESULT events.
type storageResult struct {
	Block *storage.Block      `json:"block,omitempty"`
	Metas []storage.BlockMeta `json:"metas,omitempty"`
	Error string              `json:"error,omitempty"`
}

// Adapter is the kernel.Module that routes STORAGE.* events to the GrimDB
// BlockStoreImpl. It also handles vault-level AUTH events that require
// access to the database (status check, header management).
type Adapter struct {
	db         *GrimDB
	blockStore *BlockStoreImpl
	dispatcher kernel.Dispatcher
	session    *security.SessionContext
}

// NewAdapter creates the storage adapter module.
func NewAdapter(db *GrimDB, bs *BlockStoreImpl) *Adapter {
	return &Adapter{db: db, blockStore: bs}
}

func (a *Adapter) ID() string         { return adapterModuleID }
func (a *Adapter) Channels() []string { return []string{"STORAGE"} }

// SetSession links the adapter to the global SessionContext for vault-state gating.
func (a *Adapter) SetSession(s *security.SessionContext) {
	a.session = s
}

func (a *Adapter) Start(ctx context.Context, d kernel.Dispatcher) error {
	a.dispatcher = d
	return nil
}

func (a *Adapter) Stop() error { return nil }

func (a *Adapter) Handle(e kernel.Event) error {
	switch e.Type {
	case kernel.EvStorageWrite:
		return a.handleWrite(e)
	case kernel.EvStorageRead:
		return a.handleRead(e)
	case kernel.EvStorageDelete:
		return a.handleDelete(e)
	case kernel.EvStorageList:
		return a.handleList(e)
	case kernel.EvStorageVFSMount, kernel.EvStorageReady:
		// Emitted by watchdog — no-op for the adapter.
		return nil
	case kernel.EvStorageResult:
		// Reply events reach all channel subscribers — no-op here.
		return nil
	default:
		return fmt.Errorf("storage adapter: unhandled event %s", e.Type)
	}
}

func (a *Adapter) handleWrite(e kernel.Event) error {
	if a.session != nil && !a.session.IsUnlocked() {
		return a.replyError(e, fmt.Errorf("write rejected: vault locked (no active session)"))
	}
	var p storagePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return a.replyError(e, fmt.Errorf("write: unmarshal: %w", err))
	}
	log.Printf("[adapter:WRITE] blockID=%s dataLen=%d", p.Block.ID, len(p.Block.Data))
	if err := a.blockStore.WriteBlock(p.Block); err != nil {
		log.Printf("[adapter:WRITE:FAIL] blockID=%s err=%v", p.Block.ID, err)
		return a.replyError(e, err)
	}
	log.Printf("[adapter:WRITE:OK] blockID=%s persisted", p.Block.ID)
	return a.replyOK(e, storageResult{})
}

func (a *Adapter) handleRead(e kernel.Event) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return a.replyError(e, fmt.Errorf("read: unmarshal: %w", err))
	}
	b, err := a.blockStore.ReadBlock(req.ID)
	if err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, storageResult{Block: &b})
}

func (a *Adapter) handleDelete(e kernel.Event) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return a.replyError(e, fmt.Errorf("delete: unmarshal: %w", err))
	}
	if err := a.blockStore.DeleteBlock(req.ID); err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, storageResult{})
}

func (a *Adapter) handleList(e kernel.Event) error {
	metas, err := a.blockStore.ListBlocks()
	if err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, storageResult{Metas: metas})
}

func (a *Adapter) replyOK(req kernel.Event, res storageResult) error {
	payload, _ := json.Marshal(res)
	reply := kernel.ReplyEvent(adapterModuleID, kernel.EvStorageResult, req, payload)
	reply.Timestamp = time.Now().UnixNano()
	return a.dispatcher.Dispatch(reply)
}

func (a *Adapter) replyError(req kernel.Event, err error) error {
	payload, _ := json.Marshal(storageResult{Error: err.Error()})
	reply := kernel.ReplyEvent(adapterModuleID, kernel.EvStorageResult, req, payload)
	if dErr := a.dispatcher.Dispatch(reply); dErr != nil {
		return fmt.Errorf("%w (reply dispatch failed: %v)", err, dErr)
	}
	return err
}
