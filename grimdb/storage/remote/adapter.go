//go:build enterprise

package remote

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

// BlockStoreAdapter is a kernel.Module that routes STORAGE.* events to any
// storage.BlockStore implementation. Used by the enterprise tier to route
// events to RemoteVault.
type BlockStoreAdapter struct {
	bs         storage.BlockStore
	dispatcher kernel.Dispatcher
	session    *security.SessionContext
}

// NewBlockStoreAdapter creates an adapter for the given BlockStore.
func NewBlockStoreAdapter(bs storage.BlockStore) *BlockStoreAdapter {
	return &BlockStoreAdapter{bs: bs}
}

func (a *BlockStoreAdapter) ID() string         { return "storage" }
func (a *BlockStoreAdapter) Channels() []string { return []string{"STORAGE"} }

// SetSession links the adapter to the vault-state context.
func (a *BlockStoreAdapter) SetSession(s *security.SessionContext) { a.session = s }

func (a *BlockStoreAdapter) Start(ctx context.Context, d kernel.Dispatcher) error {
	a.dispatcher = d
	return nil
}

func (a *BlockStoreAdapter) Stop() error { return nil }

func (a *BlockStoreAdapter) Handle(e kernel.Event) error {
	switch e.Type {
	case kernel.EvStorageWrite:
		return a.handleWrite(e)
	case kernel.EvStorageRead:
		return a.handleRead(e)
	case kernel.EvStorageDelete:
		return a.handleDelete(e)
	case kernel.EvStorageList:
		return a.handleList(e)
	case kernel.EvStorageVFSMount, kernel.EvStorageReady, kernel.EvStorageResult:
		return nil
	default:
		log.Printf("[bus][DEBUG] remote-adapter: no handler for %s", e.Type)
		return nil
	}
}

type remoteStoragePayload struct {
	Block storage.Block `json:"block"`
}

type remoteStorageResult struct {
	Block *storage.Block      `json:"block,omitempty"`
	Metas []storage.BlockMeta `json:"metas,omitempty"`
	Error string              `json:"error,omitempty"`
}

func (a *BlockStoreAdapter) handleWrite(e kernel.Event) error {
	if a.session != nil && !a.session.IsUnlocked() {
		return a.replyError(e, fmt.Errorf("write rejected: vault locked"))
	}
	var p remoteStoragePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return a.replyError(e, fmt.Errorf("write: unmarshal: %w", err))
	}
	if err := a.bs.WriteBlock(p.Block); err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, remoteStorageResult{})
}

func (a *BlockStoreAdapter) handleRead(e kernel.Event) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return a.replyError(e, fmt.Errorf("read: unmarshal: %w", err))
	}
	b, err := a.bs.ReadBlock(req.ID)
	if err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, remoteStorageResult{Block: &b})
}

func (a *BlockStoreAdapter) handleDelete(e kernel.Event) error {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return a.replyError(e, fmt.Errorf("delete: unmarshal: %w", err))
	}
	if err := a.bs.DeleteBlock(req.ID); err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, remoteStorageResult{})
}

func (a *BlockStoreAdapter) handleList(e kernel.Event) error {
	metas, err := a.bs.ListBlocks()
	if err != nil {
		return a.replyError(e, err)
	}
	return a.replyOK(e, remoteStorageResult{Metas: metas})
}

func (a *BlockStoreAdapter) replyOK(req kernel.Event, res remoteStorageResult) error {
	payload, _ := json.Marshal(res)
	reply := kernel.ReplyEvent("storage", kernel.EvStorageResult, req, payload)
	reply.Timestamp = time.Now().UnixNano()
	return a.dispatcher.Dispatch(reply)
}

func (a *BlockStoreAdapter) replyError(req kernel.Event, err error) error {
	payload, _ := json.Marshal(remoteStorageResult{Error: err.Error()})
	reply := kernel.ReplyEvent("storage", kernel.EvStorageResult, req, payload)
	if dErr := a.dispatcher.Dispatch(reply); dErr != nil {
		return fmt.Errorf("%w (reply dispatch failed: %v)", err, dErr)
	}
	return err
}
