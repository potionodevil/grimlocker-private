// Package sdk — Sync client built on top of GQLClient.
// Provides typed methods for vault synchronization operations.
package sdk

import (
	"context"

	"github.com/grimlocker/grimdb/engine/gql"
)

// SyncPeer represents a discovered synchronization peer.
type SyncPeer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Connected bool   `json:"connected"`
	LastSeen  int64  `json:"last_seen"`
}

// SyncStatus contains the current synchronization state and peer list.
type SyncStatus struct {
	Peers      []SyncPeer `json:"peers"`
	LastSyncAt int64      `json:"last_sync_at"`
}

// ListSyncPeers returns the current list of discovered sync peers and status.
func (c *GQLClient) ListSyncPeers(ctx context.Context) (*SyncStatus, error) {
	raw, err := c.sendCommand(ctx, "default", gql.OpSyncListPeers, nil)
	if err != nil {
		return nil, err
	}
	var status SyncStatus
	if err := unmarshalResponse(raw, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// TriggerSync initiates a full vault synchronization with all connected peers.
func (c *GQLClient) TriggerSync(ctx context.Context) error {
	_, err := c.sendCommand(ctx, "default", gql.OpSyncTrigger, nil)
	return err
}
