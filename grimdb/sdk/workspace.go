// Package sdk — Workspace client built on top of GQLClient.
// Provides typed methods for multi-tenant workspace management.
package sdk

import (
	"context"
	"fmt"

	"github.com/grimlocker/grimdb/engine/gql"
)

// Workspace represents a vault workspace.
type Workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	CreatedAt int64  `json:"created_at"`
}

// ListWorkspaces returns all available workspaces.
func (c *GQLClient) ListWorkspaces(ctx context.Context) ([]Workspace, error) {
	raw, err := c.sendCommand(ctx, "default", gql.OpWorkspaceList, nil)
	if err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := unmarshalResponse(raw, &workspaces); err != nil {
		return nil, err
	}
	return workspaces, nil
}

// CreateWorkspace creates a new workspace with the given name.
func (c *GQLClient) CreateWorkspace(ctx context.Context, name string) (*Workspace, error) {
	if name == "" {
		return nil, fmt.Errorf("sdk: workspace name cannot be empty")
	}
	payload := map[string]string{"name": name}
	raw, err := c.sendCommand(ctx, "default", gql.OpWorkspaceCreate, payload)
	if err != nil {
		return nil, err
	}
	var ws Workspace
	if err := unmarshalResponse(raw, &ws); err != nil {
		return nil, err
	}
	return &ws, nil
}

// SwitchWorkspace switches the active workspace to the given ID.
func (c *GQLClient) SwitchWorkspace(ctx context.Context, id string) error {
	payload := map[string]string{"id": id}
	_, err := c.sendCommand(ctx, "default", gql.OpWorkspaceSwitch, payload)
	return err
}

// RenameWorkspace updates the display name of a workspace.
func (c *GQLClient) RenameWorkspace(ctx context.Context, id, name string) error {
	if name == "" || len(name) > 128 {
		return fmt.Errorf("sdk: workspace name must be 1-128 characters")
	}
	payload := map[string]string{"id": id, "name": name}
	_, err := c.sendCommand(ctx, "default", gql.OpWorkspaceRename, payload)
	return err
}

// DeleteWorkspace removes a workspace and its data.
// The default workspace cannot be deleted.
func (c *GQLClient) DeleteWorkspace(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("sdk: workspace ID cannot be empty")
	}
	payload := map[string]string{"id": id}
	_, err := c.sendCommand(ctx, "default", gql.OpWorkspaceDelete, payload)
	return err
}
