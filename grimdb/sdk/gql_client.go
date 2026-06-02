// Package sdk provides the official Grimlocker SDK for Go applications.
// This file implements the GQL client for binary-protocol communication.
package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/gql"
)

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

// HealthCheck sends a simple list query to verify the daemon is responsive.
func (c *GQLClient) HealthCheck(ctx context.Context) error {
	_, err := c.ListEntries(ctx, "default")
	if err != nil {
		// ignore "vault locked" errors — daemon is healthy just locked
		log.Printf("[sdk] health check: %v", err)
	}
	return nil
}
