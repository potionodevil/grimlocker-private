// Package grimdb (adapter.go) implements the kernel.Module that bridges the
// STORAGE event channel to the underlying BlockStoreImpl.
//
// The Adapter translates STORAGE.* events into direct BlockStore calls and
// dispatches STORAGE.RESULT reply events back to the originating caller.
// It is registered on the bus during daemon startup and sits behind the
// STORAGE gate — events are silently dropped until AUTH.KEY_READY is received.
//
// Supported events:
//
//	STORAGE.WRITE  → BlockStoreImpl.WriteBlock
//	STORAGE.READ   → BlockStoreImpl.ReadBlock
//	STORAGE.DELETE → BlockStoreImpl.DeleteBlock
//	STORAGE.LIST   → BlockStoreImpl.ListBlocks
//
// Every result event (STORAGE.RESULT) includes an optional error_code field
// (from *errors.GrimlockError) so callers can distinguish vault-locked (1001)
// vs. block-not-found (2003) vs. I/O failure (2001) without string matching.
package grimdb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
	stgrimdb "github.com/grimlocker/grimdb/engine/storage/grimdb"
)

const adapterModuleID = "storage"

// adapterHandlerFn is the internal handler function type for the storage adapter registry.
type adapterHandlerFn func(kernel.Event) error

// storagePayload is the JSON schema for STORAGE.WRITE events.
type storagePayload struct {
	Block storage.Block `json:"block"`
}

// storageResult is the JSON schema for STORAGE.RESULT events.
type storageResult struct {
	Block     *storage.Block      `json:"block,omitempty"`
	Metas     []storage.BlockMeta `json:"metas,omitempty"`
	Error     string              `json:"error,omitempty"`
	ErrorCode int                 `json:"error_code,omitempty"` // GrimlockError code for structured client handling
}

// Adapter is the kernel.Module that routes STORAGE.* events to the GrimDB
// BlockStoreImpl. It also handles vault-level AUTH events that require
// access to the database (status check, header management).
type Adapter struct {
	db         *stgrimdb.GrimDB
	blockStore *stgrimdb.BlockStoreImpl
	dispatcher kernel.Dispatcher
	session    *security.SessionContext
	handlers   map[kernel.EventType]adapterHandlerFn
}

// NewAdapter creates the storage adapter module.
func NewAdapter(db *stgrimdb.GrimDB, bs *stgrimdb.BlockStoreImpl) *Adapter {
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
	a.handlers = a.buildHandlers()
	return nil
}

func (a *Adapter) Stop() error { return nil }

// buildHandlers returns the static handler registry for all STORAGE.* events.
// No-op cases are registered explicitly to silence cross-channel debug noise.
func (a *Adapter) buildHandlers() map[kernel.EventType]adapterHandlerFn {
	noop := func(kernel.Event) error { return nil }
	return map[kernel.EventType]adapterHandlerFn{
		kernel.EvStorageWrite:  a.handleWrite,
		kernel.EvStorageRead:   a.handleRead,
		kernel.EvStorageDelete: a.handleDelete,
		kernel.EvStorageList:   a.handleList,
		// Emitted by watchdog or other modules — no-ops for the adapter.
		kernel.EvStorageVFSMount: noop,
		kernel.EvStorageReady:    noop,
		// Reply events reach all channel subscribers — no-op here.
		kernel.EvStorageResult: noop,
	}
}

// Handle dispatches the event to the registered handler, or logs a structured
// debug message for unknown events instead of returning an error.
func (a *Adapter) Handle(e kernel.Event) error {
	if h, ok := a.handlers[e.Type]; ok {
		return h(e)
	}
	log.Printf("[bus][DEBUG] module=%s no_handler event=%s origin=%s", adapterModuleID, e.Type, e.Origin)
	return nil
}

func (a *Adapter) handleWrite(e kernel.Event) error {
	if a.session != nil && !a.session.IsUnlocked() {
		return a.replyError(e, gerrors.NewVaultLockedError())
	}
	var p storagePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return a.replyError(e, gerrors.NewProtocolError("write_unmarshal", err))
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
		return a.replyError(e, gerrors.NewProtocolError("read_unmarshal", err))
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
		return a.replyError(e, gerrors.NewProtocolError("delete_unmarshal", err))
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
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	payload, _ := json.Marshal(storageResult{Error: err.Error(), ErrorCode: code})
	reply := kernel.ReplyEvent(adapterModuleID, kernel.EvStorageResult, req, payload)
	if dErr := a.dispatcher.Dispatch(reply); dErr != nil {
		return fmt.Errorf("%w (reply dispatch failed: %v)", err, dErr)
	}
	return err
}
