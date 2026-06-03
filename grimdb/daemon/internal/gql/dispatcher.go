package gql

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	egql "github.com/grimlocker/grimdb/engine/gql"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
)

const gqlRequestTimeout = 30 * time.Second
const defaultLimit = 50

// Dispatcher maps validated GQL queries to kernel Events and returns GQL results.
// No business logic — purely a translation layer between GQL operations and the
// existing kernel event types (STORAGE.*, ENTRY.*).
type Dispatcher struct {
	bus             kernel.Dispatcher
	blockStore      storage.BlockStore
	entryDispatcher kernel.Dispatcher // for ENTRY.* events (may be same as bus)
}

// NewDispatcher creates a GQL Dispatcher.
func NewDispatcher(bus kernel.Dispatcher, bs storage.BlockStore) *Dispatcher {
	return &Dispatcher{
		bus:             bus,
		blockStore:      bs,
		entryDispatcher: bus,
	}
}

// Dispatch executes a validated egql.GQLQuery and returns the result.
// The query has already passed syntactic and semantic validation.
func (d *Dispatcher) Dispatch(ctx context.Context, query *egql.GQLQuery) (*egql.GQLResult, error) {
	switch query.Operation {
	case egql.OpListEntries:
		return d.handleList(ctx, query)
	case egql.OpGetEntry:
		return d.handleGet(ctx, query)
	case egql.OpQueryEntries:
		return d.handleQuery(ctx, query)
	case egql.OpCreateEntry:
		return d.handleCreate(ctx, query)
	case egql.OpUpdateEntry:
		return d.handleUpdate(ctx, query)
	case egql.OpDeleteEntry:
		return d.handleDelete(ctx, query)
	default:
		return nil, fmt.Errorf("gql: unknown operation %q", query.Operation)
	}
}

func (d *Dispatcher) handleList(ctx context.Context, q *egql.GQLQuery) (*egql.GQLResult, error) {
	ev := kernel.NewEvent("gql", kernel.EvStorageList, nil)
	reqCtx, cancel := context.WithTimeout(ctx, gqlRequestTimeout)
	defer cancel()

	result, err := d.bus.Request(reqCtx, ev)
	if err != nil {
		return &egql.GQLResult{Success: false, ErrorCode: -1, ErrorMsg: fmt.Sprintf("STORAGE.LIST timeout: %v", err)}, nil
	}

	var stored struct {
		Metas []storage.BlockMeta `json:"metas,omitempty"`
		Error string              `json:"error,omitempty"`
	}
	if err := json.Unmarshal(result.Payload, &stored); err != nil {
		log.Printf("[gql:list] unmarshal error: %v", err)
		return &egql.GQLResult{Success: false, ErrorCode: -2, ErrorMsg: "invalid storage response"}, nil
	}
	if stored.Error != "" {
		return &egql.GQLResult{Success: false, ErrorCode: -3, ErrorMsg: stored.Error}, nil
	}

	entries := make([]egql.GQLEntry, 0, len(stored.Metas))
	for _, meta := range stored.Metas {
		entries = append(entries, egql.GQLEntry{
			ID:        meta.ID,
			Category:  string(meta.Category),
			CreatedAt: meta.CreatedAt,
			UpdatedAt: meta.UpdatedAt,
		})
	}

	// Apply pagination
	total := uint32(len(entries))
	if q.Limit == 0 {
		q.Limit = defaultLimit
	}
	start := q.Offset
	if start >= total {
		start = total
	}
	end := start + q.Limit
	if end > total {
		end = total
	}

	return &egql.GQLResult{
		Success:    true,
		Entries:    entries[start:end],
		TotalCount: total,
	}, nil
}

func (d *Dispatcher) handleGet(ctx context.Context, q *egql.GQLQuery) (*egql.GQLResult, error) {
	if q.EntryID == "" {
		return &egql.GQLResult{Success: false, ErrorCode: -10, ErrorMsg: "entry_id required for get_entry"}, nil
	}

	reqPayload, _ := json.Marshal(map[string]string{"id": q.EntryID})
	ev := kernel.NewEvent("gql", kernel.EvStorageRead, reqPayload)
	reqCtx, cancel := context.WithTimeout(ctx, gqlRequestTimeout)
	defer cancel()

	result, err := d.bus.Request(reqCtx, ev)
	if err != nil {
		return &egql.GQLResult{Success: false, ErrorCode: -1, ErrorMsg: fmt.Sprintf("STORAGE.READ timeout: %v", err)}, nil
	}

	var stored struct {
		Block *storage.Block `json:"block,omitempty"`
		Error string         `json:"error,omitempty"`
	}
	json.Unmarshal(result.Payload, &stored)
	if stored.Error != "" || stored.Block == nil {
		msg := "entry not found"
		if stored.Error != "" {
			msg = stored.Error
		}
		return &egql.GQLResult{Success: false, ErrorCode: -11, ErrorMsg: msg}, nil
	}

	entry := egql.GQLEntry{
		ID:        stored.Block.ID,
		CreatedAt: stored.Block.CreatedAt,
		UpdatedAt: stored.Block.UpdatedAt,
	}
	return &egql.GQLResult{
		Success:    true,
		Entries:    []egql.GQLEntry{entry},
		TotalCount: 1,
	}, nil
}

func (d *Dispatcher) handleQuery(ctx context.Context, q *egql.GQLQuery) (*egql.GQLResult, error) {
	var cat storage.Category
	if q.Category != "" {
		cat = storage.Category(q.Category)
	}

	metas, err := d.blockStore.QueryBlocks(cat)
	if err != nil {
		return &egql.GQLResult{Success: false, ErrorCode: -20, ErrorMsg: err.Error()}, nil
	}

	entries := make([]egql.GQLEntry, 0, len(metas))
	for _, meta := range metas {
		entries = append(entries, egql.GQLEntry{
			ID:        meta.ID,
			Category:  string(meta.Category),
			CreatedAt: meta.CreatedAt,
			UpdatedAt: meta.UpdatedAt,
		})
	}

	total := uint32(len(entries))
	if q.Limit == 0 {
		q.Limit = defaultLimit
	}
	start := q.Offset
	if start >= total {
		start = total
	}
	end := start + q.Limit
	if end > total {
		end = total
	}

	return &egql.GQLResult{
		Success:    true,
		Entries:    entries[start:end],
		TotalCount: total,
	}, nil
}

func (d *Dispatcher) handleCreate(ctx context.Context, q *egql.GQLQuery) (*egql.GQLResult, error) {
	evPayload, _ := json.Marshal(map[string]interface{}{
		"subject_id": q.Namespace,
		"title":      q.Title,
		"type":       q.Category,
		"fields":     q.Fields,
	})
	ev := kernel.NewEvent("gql", kernel.EvEntryCreate, evPayload)
	reqCtx, cancel := context.WithTimeout(ctx, gqlRequestTimeout)
	defer cancel()

	result, err := d.bus.Request(reqCtx, ev)
	if err != nil {
		return &egql.GQLResult{Success: false, ErrorCode: -1, ErrorMsg: fmt.Sprintf("ENTRY.CREATE timeout: %v", err)}, nil
	}

	var res struct {
		Error string `json:"error,omitempty"`
		ID    string `json:"id,omitempty"`
	}
	json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		return &egql.GQLResult{Success: false, ErrorCode: -30, ErrorMsg: res.Error}, nil
	}

	return &egql.GQLResult{
		Success: true,
		Entries: []egql.GQLEntry{{ID: res.ID, Title: q.Title, Category: q.Category, Fields: q.Fields}},
	}, nil
}

func (d *Dispatcher) handleUpdate(ctx context.Context, q *egql.GQLQuery) (*egql.GQLResult, error) {
	if q.EntryID == "" {
		return &egql.GQLResult{Success: false, ErrorCode: -10, ErrorMsg: "entry_id required for update_entry"}, nil
	}

	evPayload, _ := json.Marshal(map[string]interface{}{
		"subject_id": q.Namespace,
		"id":         q.EntryID,
		"title":      q.Title,
		"fields":     q.Fields,
	})
	ev := kernel.NewEvent("gql", kernel.EvEntryUpdate, evPayload)
	reqCtx, cancel := context.WithTimeout(ctx, gqlRequestTimeout)
	defer cancel()

	result, err := d.bus.Request(reqCtx, ev)
	if err != nil {
		return &egql.GQLResult{Success: false, ErrorCode: -1, ErrorMsg: fmt.Sprintf("ENTRY.UPDATE timeout: %v", err)}, nil
	}

	var res struct {
		Error string `json:"error,omitempty"`
	}
	json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		return &egql.GQLResult{Success: false, ErrorCode: -31, ErrorMsg: res.Error}, nil
	}

	return &egql.GQLResult{
		Success: true,
		Entries: []egql.GQLEntry{{ID: q.EntryID, Title: q.Title, Category: q.Category, Fields: q.Fields}},
	}, nil
}

func (d *Dispatcher) handleDelete(ctx context.Context, q *egql.GQLQuery) (*egql.GQLResult, error) {
	if q.EntryID == "" {
		return &egql.GQLResult{Success: false, ErrorCode: -10, ErrorMsg: "entry_id required for delete_entry"}, nil
	}

	evPayload, _ := json.Marshal(map[string]interface{}{
		"subject_id": q.Namespace,
		"id":         q.EntryID,
	})
	ev := kernel.NewEvent("gql", kernel.EvEntryDelete, evPayload)
	reqCtx, cancel := context.WithTimeout(ctx, gqlRequestTimeout)
	defer cancel()

	result, err := d.bus.Request(reqCtx, ev)
	if err != nil {
		return &egql.GQLResult{Success: false, ErrorCode: -1, ErrorMsg: fmt.Sprintf("ENTRY.DELETE timeout: %v", err)}, nil
	}

	var res struct {
		Error string `json:"error,omitempty"`
	}
	json.Unmarshal(result.Payload, &res)
	if res.Error != "" {
		return &egql.GQLResult{Success: false, ErrorCode: -32, ErrorMsg: res.Error}, nil
	}

	return &egql.GQLResult{Success: true}, nil
}
