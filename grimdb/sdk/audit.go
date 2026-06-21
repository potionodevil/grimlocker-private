// Package sdk — Audit client built on top of GQLClient.
// Provides typed methods for security event auditing.
package sdk

import (
	"context"

	"github.com/grimlocker/grimdb/engine/gql"
)

// AuditEvent is an immutable audit record with cryptographic chaining.
type AuditEvent struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
	SubjectID string `json:"subject_id"`
}

// ListAuditEvents returns the most recent n audit events.
func (c *GQLClient) ListAuditEvents(ctx context.Context, n int) ([]AuditEvent, error) {
	if n <= 0 {
		n = 50
	}
	payload := map[string]int{"n": n}
	raw, err := c.sendCommand(ctx, "default", gql.OpAuditList, payload)
	if err != nil {
		return nil, err
	}
	var events []AuditEvent
	if err := unmarshalResponse(raw, &events); err != nil {
		return nil, err
	}
	return events, nil
}
