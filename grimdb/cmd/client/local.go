package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// localConn implements daemonConn via WebSocket to a locally-running daemon.
type localConn struct {
	baseURL string
	token   string
	client  *http.Client
}

// newLocalConn creates a connection to a local daemon at the given address.
// address is in the form "127.0.0.1:PORT" or a full ws:// URL.
func newLocalConn(address, token string) *localConn {
	// Normalise to http:// for REST calls.
	base := address
	if !strings.HasPrefix(base, "http") {
		base = "http://" + base
	}
	base = strings.TrimSuffix(base, "/ws")
	return &localConn{
		baseURL: base,
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Send posts an action to the daemon via the REST API endpoint.
func (c *localConn) Send(eventType string, payload []byte) ([]byte, error) {
	body, err := json.Marshal(map[string]interface{}{
		"action":  eventType,
		"payload": json.RawMessage(payload),
	})
	if err != nil {
		return nil, err
	}

	apiURL := c.baseURL + "/api/v1"
	req, err := http.NewRequest(http.MethodPost, apiURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Grimlocker-Token", c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send %s: %w", eventType, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned %d: %s", resp.StatusCode, respBody)
	}
	return respBody, nil
}

// postRaw sends a raw POST request to the given path (e.g. "/init").
func (c *localConn) postRaw(path string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Health queries the daemon's /health endpoint.
func (c *localConn) Health() ([]byte, error) {
	healthURL := c.baseURL + "/health"
	resp, err := c.client.Get(healthURL)
	if err != nil {
		return nil, fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// Close is a no-op for the HTTP client.
func (c *localConn) Close() error { return nil }

// discoverLocalDaemon reads the daemon's IPC address from a well-known location.
// The daemon writes its address to stdout on startup as GRIMLOCKER_IPC=ws://...
// For the CLI, the user can set GRIMLOCKER_IPC env var or provide --addr flag.
func discoverLocalDaemon(addr string) string {
	if addr != "" {
		return addr
	}
	// Default local address used by the daemon.
	return "127.0.0.1:0"
}

// parseWSToHTTP converts ws://host:port/ws to http://host:port
func parseWSToHTTP(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil {
		return wsURL
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}
	u.Path = ""
	return u.String()
}
