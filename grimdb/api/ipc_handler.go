// Package api provides the JSON-over-HTTP entry point for SDK clients and tooling.
// This is distinct from the binary WebSocket protocol in translator.go, which serves
// the Tauri frontend. Channel-restriction is enforced as middleware: requests targeting
// SECURITY or CRYPTO channels are rejected with 403 Forbidden.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	gerrors "github.com/grimlocker/grimdb/errors"
	"github.com/grimlocker/grimdb/kernel"
)

// actionMap translates JSON action names to kernel EventType values.
// Only actions in this table are accepted; all others return 400.
var actionMap = map[string]kernel.EventType{
	// Auth / session
	"vault.unlock": kernel.EvAuthUnlock,
	"vault.init":   kernel.EvAuthSetup,
	"vault.status": kernel.EvAuthStatus,
	"vault.logout": kernel.EvAuthLogout,

	// Raw block storage (low-level)
	"storage.write":  kernel.EvStorageWrite,
	"storage.read":   kernel.EvStorageRead,
	"storage.delete": kernel.EvStorageDelete,
	"storage.list":   kernel.EvStorageList,

	// High-level vault entries (CLI and REST clients)
	"entry.create": kernel.EvEntryCreate,
	"entry.read":   kernel.EvEntryRead,
	"entry.update": kernel.EvEntryUpdate,
	"entry.delete": kernel.EvEntryDelete,
	"entry.query":  kernel.EvEntryQuery,

	"sync.begin": kernel.EvSyncBegin,
}

// blockedChannels are channels that the UI and SDK may never dispatch to directly.
// Requests resolving to these channels are rejected with 403 Forbidden.
var blockedChannels = map[string]bool{
	"SECURITY": true,
	"CRYPTO":   true,
}

// Handler is the JSON IPC entry point. Mount on /api/v1 of the IPC mux.
type Handler struct {
	bus kernel.Dispatcher
}

// NewHandler creates a Handler backed by the given dispatcher.
func NewHandler(bus kernel.Dispatcher) *Handler {
	return &Handler{bus: bus}
}

// ipcRequest is the JSON body accepted by ServeHTTP.
type ipcRequest struct {
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ipcResponse is the JSON body written by ServeHTTP.
type ipcResponse struct {
	OK        bool            `json:"ok"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
	ErrorCode int             `json:"error_code,omitempty"` // GrimlockError code for SDK clients
}

// ServeHTTP reads {"action":"...","payload":{...}}, validates channel restrictions,
// dispatches to the bus, and writes the result as JSON. It enforces a 30s timeout
// per request to match the WebSocket translator behaviour.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "method not allowed"})
		return
	}

	var req ipcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "invalid JSON body"})
		return
	}

	evType, ok := actionMap[req.Action]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "unknown action"})
		return
	}

	if blockedChannels[evType.Channel()] {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: "channel restricted"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	payload := []byte(req.Payload)
	if len(payload) == 0 {
		payload = []byte("{}")
	}

	ev := kernel.NewEvent("ipc-handler", evType, payload)
	reply, err := h.bus.Request(ctx, ev)
	if err != nil {
		// If the error is a typed GrimlockError, use its HTTP status and code.
		status := http.StatusInternalServerError
		code := 0
		if ge, ok := err.(*gerrors.GrimlockError); ok {
			status = ge.HTTPStatus()
			code = ge.Code
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(ipcResponse{Error: err.Error(), ErrorCode: code})
		return
	}

	_ = json.NewEncoder(w).Encode(ipcResponse{OK: true, Payload: reply.Payload})
}
