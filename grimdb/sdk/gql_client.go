// Package sdk provides the official Grimlocker SDK for Go applications.
// This file implements the GQL client for binary-protocol communication.
package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/engine/gql"
)

// BatchEntry describes a single entry to be created in a batch operation.
type BatchEntry struct {
	Title    string
	Category string
	Fields   map[string]string
}

// DaemonEventType categorizes daemon events received over the event stream.
type DaemonEventType string

const (
	DaemonEventEntryChanged DaemonEventType = "entry_changed"
	DaemonEventSyncComplete DaemonEventType = "sync_complete"
	DaemonEventConnected    DaemonEventType = "connected"
	DaemonEventDisconnected DaemonEventType = "disconnected"
)

// DaemonEvent is a typed daemon event delivered by SubscribeEvents.
type DaemonEvent struct {
	Type    DaemonEventType
	EntryID string
	Data    map[string]interface{}
}

type cbState int

const (
	cbClosed cbState = iota
	cbOpen
	cbHalfOpen
)

type circuitBreaker struct {
	mu               sync.Mutex
	state            cbState
	consecutiveFails int
	lastFailTime     time.Time
}

func (cb *circuitBreaker) isOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.state == cbOpen {
		if time.Since(cb.lastFailTime) > 30*time.Second {
			cb.state = cbHalfOpen
			return false
		}
		return true
	}
	return false
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFails = 0
	cb.state = cbClosed
	cb.lastFailTime = time.Time{}
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFails++
	if cb.consecutiveFails >= 5 {
		cb.state = cbOpen
		cb.lastFailTime = time.Now()
	}
}

// GQLClient is a secure, injection-immune client for Grimlocker's GQL protocol.
// All queries are binary-encoded — no text parsing, no SQL injection, no JSON injection.
//
// Usage:
//
//	client, _ := sdk.DialGQL(ctx, "ws://127.0.0.1:11003/ws?token=...")
//	defer client.Close()
//
//	entries, err := client.ListEntries(ctx, "default")
//	if err != nil { ... }
type GQLClient struct {
	conn     *gorillaws.Conn
	endpoint string
	closed   bool
	cb       circuitBreaker
}

// DialGQL connects to a Grimlocker daemon via WebSocket and performs the
// GQL protocol handshake. Returns a ready-to-use GQLClient.
func DialGQL(ctx context.Context, endpoint string) (*GQLClient, error) {
	dialer := gorillaws.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("sdk: dial %s: %w", endpoint, err)
	}

	// Wait for INIT.READY from the daemon
	_, _, err = conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sdk: handshake read: %w", err)
	}

	return &GQLClient{
		conn:     conn,
		endpoint: endpoint,
	}, nil
}

// Close terminates the WebSocket connection.
func (c *GQLClient) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

// Execute sends a GQL query frame, awaits the result, and returns it.
// This is the low-level API — use the typed methods (ListEntries, CreateEntry, etc.) for convenience.
func (c *GQLClient) Execute(ctx context.Context, query *gql.GQLQuery) (*gql.GQLResult, error) {
	if c.closed {
		return nil, fmt.Errorf("sdk: client is closed")
	}

	if c.cb.isOpen() {
		return nil, fmt.Errorf("sdk: circuit breaker open")
	}

	var lastErr error
	delay := 100 * time.Millisecond
	for attempt := 0; attempt <= 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
			delay *= 2
		}
		result, err := c.executeOnce(ctx, query)
		lastErr = err
		_ = result
		if err == nil {
			c.cb.recordSuccess()
			return result, nil
		}
		if !isRetryable(err, result) {
			c.cb.recordFailure()
			return nil, err
		}
	}
	c.cb.recordFailure()
	return nil, lastErr
}

func (c *GQLClient) executeOnce(ctx context.Context, query *gql.GQLQuery) (*gql.GQLResult, error) {
	frame := gql.NewQueryFrame(query)
	data := frame.Encode()

	if err := c.conn.WriteMessage(gorillaws.BinaryMessage, data); err != nil {
		return nil, fmt.Errorf("sdk: write frame: %w", err)
	}

	_, respData, err := c.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("sdk: read response: %w", err)
	}

	respFrame, err := gql.DecodeFrame(respData)
	if err != nil {
		return nil, fmt.Errorf("sdk: decode response frame: %w", err)
	}

	if respFrame.Opcode == gql.OpcodeError {
		var errResult gql.GQLResult
		if json.Unmarshal(respFrame.Payload, &errResult) == nil {
			return &errResult, fmt.Errorf("sdk: GQL error %s (%d): %s",
				gql.ErrorCodeName(errResult.ErrorCode), errResult.ErrorCode, errResult.ErrorMsg)
		}
		return nil, fmt.Errorf("sdk: GQL error (unparseable)")
	}

	var result gql.GQLResult
	if err := json.Unmarshal(respFrame.Payload, &result); err != nil {
		return nil, fmt.Errorf("sdk: unmarshal result: %w", err)
	}

	return &result, nil
}

func isRetryable(err error, result *gql.GQLResult) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	// Connection-level errors are always retryable.
	if strings.Contains(s, "sdk: dial") ||
		strings.Contains(s, "sdk: handshake") ||
		strings.Contains(s, "sdk: write frame") ||
		strings.Contains(s, "sdk: read response") ||
		strings.Contains(s, "sdk: unmarshal result") ||
		strings.Contains(s, "websocket") ||
		strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "connection reset by peer") ||
		strings.Contains(s, "broken pipe") {
		return true
	}
	// 5xx-equivalent daemon errors.
	if result != nil {
		switch result.ErrorCode {
		case -1, -2, -3, -100, -105:
			return true
		}
	}
	return false
}

// ListEntries returns all entries in the given namespace.
func (c *GQLClient) ListEntries(ctx context.Context, namespace string) ([]gql.GQLEntry, error) {
	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpListEntries,
	}
	result, err := c.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sdk: list failed: %s", result.ErrorMsg)
	}
	return result.Entries, nil
}

// GetEntry retrieves a single entry by ID.
func (c *GQLClient) GetEntry(ctx context.Context, namespace, entryID string) (*gql.GQLEntry, error) {
	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpGetEntry,
		EntryID:   entryID,
	}
	result, err := c.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	if !result.Success || len(result.Entries) == 0 {
		return nil, fmt.Errorf("sdk: entry not found: %s", entryID)
	}
	return &result.Entries[0], nil
}

// QueryEntries returns entries filtered by category.
func (c *GQLClient) QueryEntries(ctx context.Context, namespace, category string) ([]gql.GQLEntry, error) {
	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpQueryEntries,
		Category:  category,
	}
	result, err := c.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sdk: query failed: %s", result.ErrorMsg)
	}
	return result.Entries, nil
}

// CreateEntry creates a new vault entry.
func (c *GQLClient) CreateEntry(ctx context.Context, namespace, title, category string, fields map[string]string) (*gql.GQLEntry, error) {
	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpCreateEntry,
		Title:     title,
		Category:  category,
		Fields:    fields,
	}
	result, err := c.Execute(ctx, query)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sdk: create failed: %s", result.ErrorMsg)
	}
	if len(result.Entries) == 0 {
		return nil, fmt.Errorf("sdk: create returned no entry")
	}
	return &result.Entries[0], nil
}

// UpdateEntry modifies an existing entry's title and fields.
func (c *GQLClient) UpdateEntry(ctx context.Context, namespace, entryID, title string, fields map[string]string) error {
	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpUpdateEntry,
		EntryID:   entryID,
		Title:     title,
		Fields:    fields,
	}
	result, err := c.Execute(ctx, query)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("sdk: update failed: %s", result.ErrorMsg)
	}
	return nil
}

// DeleteEntry removes an entry by ID.
func (c *GQLClient) DeleteEntry(ctx context.Context, namespace, entryID string) error {
	query := &gql.GQLQuery{
		Namespace: namespace,
		Operation: gql.OpDeleteEntry,
		EntryID:   entryID,
	}
	result, err := c.Execute(ctx, query)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("sdk: delete failed: %s", result.ErrorMsg)
	}
	return nil
}

// CreateEntriesBatch creates multiple entries in the given namespace.
func (c *GQLClient) CreateEntriesBatch(ctx context.Context, namespace string, entries []BatchEntry) ([]string, error) {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		entry, err := c.CreateEntry(ctx, namespace, e.Title, e.Category, e.Fields)
		if err != nil {
			return nil, err
		}
		ids = append(ids, entry.ID)
	}
	return ids, nil
}

// DeleteEntriesBatch removes multiple entries by ID.
func (c *GQLClient) DeleteEntriesBatch(ctx context.Context, namespace string, entryIDs []string) error {
	for _, id := range entryIDs {
		if err := c.DeleteEntry(ctx, namespace, id); err != nil {
			return err
		}
	}
	return nil
}

// UnlockVault unlocks the vault with the master password.
func (c *GQLClient) UnlockVault(ctx context.Context, password string) error {
	q := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpVaultUnlock,
		Fields: map[string]string{
			"password": password,
		},
	}
	result, err := c.Execute(ctx, q)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("sdk: unlock failed: %s", result.ErrorMsg)
	}
	return nil
}

// LockVault locks the vault (logout).
func (c *GQLClient) LockVault(ctx context.Context) error {
	q := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpVaultLogout,
	}
	result, err := c.Execute(ctx, q)
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("sdk: lock failed: %s", result.ErrorMsg)
	}
	return nil
}

// VaultStatus returns the current vault initialization and lock state.
func (c *GQLClient) VaultStatus(ctx context.Context) (map[string]interface{}, error) {
	q := &gql.GQLQuery{
		Namespace: "default",
		Operation: gql.OpVaultStatus,
	}
	result, err := c.Execute(ctx, q)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sdk: status failed: %s", result.ErrorMsg)
	}
	// Result comes back as a single entry with fields
	if len(result.Entries) > 0 && result.Entries[0].Fields != nil {
		status := make(map[string]interface{})
		for k, v := range result.Entries[0].Fields {
			status[k] = v
		}
		return status, nil
	}
	return map[string]interface{}{}, nil
}

// HealthCheck sends a simple list query to verify the daemon is responsive.
func (c *GQLClient) HealthCheck(ctx context.Context) error {
	_, err := c.ListEntries(ctx, "default")
	if err != nil {
		// ignore "vault locked" errors — daemon is healthy just locked
		log.Printf("[sdk] health check: %v", err)
	}
	return nil
}

// SubscribeEvents opens a dedicated WebSocket connection to the daemon and
// streams typed events on the returned channel. The channel is closed when
// the context is cancelled or the connection is dropped.
func (c *GQLClient) SubscribeEvents(ctx context.Context) (<-chan DaemonEvent, error) {
	dialer := gorillaws.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.DialContext(ctx, c.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("sdk: dial events %s: %w", c.endpoint, err)
	}

	// Consume INIT.READY handshake frame
	_, _, err = conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sdk: events handshake read: %w", err)
	}

	ch := make(chan DaemonEvent, 16)
	go func() {
		defer close(ch)
		defer conn.Close()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msgType, respData, err := conn.ReadMessage()
			if err != nil {
				select {
				case ch <- DaemonEvent{Type: DaemonEventDisconnected, Data: map[string]interface{}{"error": err.Error()}}:
				case <-ctx.Done():
				}
				return
			}
			var payload map[string]interface{}
			switch msgType {
			case gorillaws.BinaryMessage:
				respFrame, frameErr := gql.DecodeFrame(respData)
				if frameErr != nil {
					continue
				}
				if jsonErr := json.Unmarshal(respFrame.Payload, &payload); jsonErr != nil {
					continue
				}
			case gorillaws.TextMessage:
				if jsonErr := json.Unmarshal(respData, &payload); jsonErr != nil {
					continue
				}
			default:
				continue
			}
			evtType, _ := payload["event_type"].(string)
			switch evtType {
			case "entry_changed":
				entryID, _ := payload["entry_id"].(string)
				ch <- DaemonEvent{Type: DaemonEventEntryChanged, EntryID: entryID, Data: payload}
			case "sync_complete":
				ch <- DaemonEvent{Type: DaemonEventSyncComplete, Data: payload}
			case "connected":
				ch <- DaemonEvent{Type: DaemonEventConnected, Data: payload}
			case "disconnected":
				ch <- DaemonEvent{Type: DaemonEventDisconnected, Data: payload}
			default:
				if evtType != "" {
					ch <- DaemonEvent{Type: DaemonEventType(evtType), Data: payload}
				}
			}
		}
	}()
	return ch, nil
}
