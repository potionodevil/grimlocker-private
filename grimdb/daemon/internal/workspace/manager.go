package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Workspace represents a single vault workspace (multi-tenant).
type Workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Dir       string `json:"dir"`       // Absolute path to workspace directory
	CreatedAt int64  `json:"created_at"`
	IsDefault bool   `json:"is_default"`
}

// WorkspaceRegistry is the JSON schema stored in workspaces.json
type WorkspaceRegistry struct {
	ActiveID   string       `json:"active_id"`
	Workspaces []*Workspace `json:"workspaces"`
}

// WorkspaceManager handles workspace lifecycle operations.
type WorkspaceManager struct {
	baseDir    string
	mu         sync.RWMutex
	active     *Workspace
	workspaces map[string]*Workspace // ID → Workspace
}

const (
	WorkspacesFileName = "workspaces.json"
	DefaultWorkspaceID = "default"
)

// NewWorkspaceManager initializes the workspace manager and loads or creates the default workspace.
func NewWorkspaceManager(baseDir string) (*WorkspaceManager, error) {
	wm := &WorkspaceManager{
		baseDir:    baseDir,
		workspaces: make(map[string]*Workspace),
	}

	// Try to load existing workspaces
	if err := wm.load(); err != nil {
		// If no workspaces exist, create the default one
		defaultWS := &Workspace{
			ID:        DefaultWorkspaceID,
			Name:      "Default",
			Dir:       filepath.Join(baseDir, "workspace_"+DefaultWorkspaceID),
			CreatedAt: time.Now().UnixNano(),
			IsDefault: true,
		}

		// Ensure the directory exists
		if err := os.MkdirAll(defaultWS.Dir, 0700); err != nil {
			return nil, fmt.Errorf("create default workspace dir: %w", err)
		}

		wm.workspaces[DefaultWorkspaceID] = defaultWS
		wm.active = defaultWS

		// Save the new workspace registry
		if err := wm.save(); err != nil {
			return nil, fmt.Errorf("save workspace registry: %w", err)
		}
	}

	return wm, nil
}

// List returns all workspaces.
func (wm *WorkspaceManager) List() []*Workspace {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	result := make([]*Workspace, 0, len(wm.workspaces))
	for _, ws := range wm.workspaces {
		result = append(result, ws)
	}
	return result
}

// Active returns the currently active workspace.
func (wm *WorkspaceManager) Active() *Workspace {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.active
}

// Create creates a new workspace with the given name.
func (wm *WorkspaceManager) Create(name string) (*Workspace, error) {
	if name == "" {
		return nil, fmt.Errorf("workspace name cannot be empty")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Generate a unique ID
	id := uuid.New().String()[:8] // Use first 8 characters of UUID for brevity
	wsDir := filepath.Join(wm.baseDir, fmt.Sprintf("workspace_%s", id))

	// Create the workspace directory
	if err := os.MkdirAll(wsDir, 0700); err != nil {
		return nil, fmt.Errorf("create workspace dir: %w", err)
	}

	ws := &Workspace{
		ID:        id,
		Name:      name,
		Dir:       wsDir,
		CreatedAt: time.Now().UnixNano(),
		IsDefault: false,
	}

	wm.workspaces[id] = ws

	// Persist changes
	if err := wm.saveLocked(); err != nil {
		// Rollback directory creation
		_ = os.RemoveAll(wsDir)
		delete(wm.workspaces, id)
		return nil, fmt.Errorf("save workspace registry: %w", err)
	}

	return ws, nil
}

// Switch activates a workspace by ID.
func (wm *WorkspaceManager) Switch(id string) (*Workspace, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	ws, ok := wm.workspaces[id]
	if !ok {
		return nil, fmt.Errorf("workspace not found: %s", id)
	}

	wm.active = ws

	// Persist the new active workspace
	if err := wm.saveLocked(); err != nil {
		// Revert on failure
		if wm.active != ws {
			return nil, err
		}
		return nil, fmt.Errorf("save workspace registry: %w", err)
	}

	return ws, nil
}

// Delete removes a workspace and its data. The default workspace cannot be deleted.
func (wm *WorkspaceManager) Delete(id string) error {
	if id == DefaultWorkspaceID {
		return fmt.Errorf("cannot delete the default workspace")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	ws, ok := wm.workspaces[id]
	if !ok {
		return fmt.Errorf("workspace not found: %s", id)
	}

	// If this is the active workspace, switch to default
	if wm.active.ID == id {
		wm.active = wm.workspaces[DefaultWorkspaceID]
	}

	// Remove from map
	delete(wm.workspaces, id)

	// Delete the workspace directory
	if err := os.RemoveAll(ws.Dir); err != nil {
		// Restore to map on failure
		wm.workspaces[id] = ws
		return fmt.Errorf("delete workspace dir: %w", err)
	}

	// Persist changes
	if err := wm.saveLocked(); err != nil {
		// Restore to map on failure
		wm.workspaces[id] = ws
		return fmt.Errorf("save workspace registry: %w", err)
	}

	return nil
}

// Rename updates the display name of a workspace. The ID and directory remain
// unchanged — only the human-readable name is modified.
func (wm *WorkspaceManager) Rename(id, newName string) error {
	if newName == "" || len(newName) > 128 {
		return fmt.Errorf("workspace name must be 1-128 characters")
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	ws, ok := wm.workspaces[id]
	if !ok {
		return fmt.Errorf("workspace not found: %s", id)
	}

	oldName := ws.Name
	ws.Name = newName

	if err := wm.saveLocked(); err != nil {
		ws.Name = oldName // rollback
		return fmt.Errorf("save workspace registry: %w", err)
	}

	return nil
}

// GetWorkspaceDir returns the directory path for a given workspace.
func (wm *WorkspaceManager) GetWorkspaceDir(id string) (string, error) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	ws, ok := wm.workspaces[id]
	if !ok {
		return "", fmt.Errorf("workspace not found: %s", id)
	}

	return ws.Dir, nil
}

// load reads the workspace registry from disk.
func (wm *WorkspaceManager) load() error {
	registryPath := filepath.Join(wm.baseDir, WorkspacesFileName)

	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no workspace registry found")
		}
		return fmt.Errorf("read registry: %w", err)
	}

	var registry WorkspaceRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return fmt.Errorf("unmarshal registry: %w", err)
	}

	// Populate the workspaces map
	for _, ws := range registry.Workspaces {
		wm.workspaces[ws.ID] = ws
	}

	// Find and set the active workspace
	if registry.ActiveID != "" {
		if active, ok := wm.workspaces[registry.ActiveID]; ok {
			wm.active = active
		} else {
			// Fallback to default if active doesn't exist
			wm.active = wm.workspaces[DefaultWorkspaceID]
		}
	} else if defaultWS, ok := wm.workspaces[DefaultWorkspaceID]; ok {
		wm.active = defaultWS
	}

	return nil
}

// save persists the workspace registry to disk.
func (wm *WorkspaceManager) save() error {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.saveLocked()
}

// saveLocked persists the workspace registry. Must be called with lock held.
func (wm *WorkspaceManager) saveLocked() error {
	registry := WorkspaceRegistry{
		Workspaces: make([]*Workspace, 0, len(wm.workspaces)),
	}

	if wm.active != nil {
		registry.ActiveID = wm.active.ID
	}

	// Collect all workspaces
	for _, ws := range wm.workspaces {
		registry.Workspaces = append(registry.Workspaces, ws)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(&registry, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal registry: %w", err)
	}

	// Write atomically with temp file
	registryPath := filepath.Join(wm.baseDir, WorkspacesFileName)
	tmpPath := registryPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write temp registry: %w", err)
	}

	if err := os.Rename(tmpPath, registryPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename registry: %w", err)
	}

	return nil
}
