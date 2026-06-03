package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// remoteConn implements daemonConn via mTLS to an enterprise daemon.
type remoteConn struct {
	baseURL string
	client  *http.Client
}

// newRemoteConn creates an mTLS connection to the enterprise daemon at daemonAddr.
// certPath/keyPath are the client certificate and private key files.
// caPath is the CA certificate that signed both client and server certificates.
func newRemoteConn(daemonAddr, certPath, keyPath, caPath string) (*remoteConn, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert %s: %w", caPath, err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid CA certificates in %s", caPath)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}

	base := daemonAddr
	if !strings.HasPrefix(base, "https://") && !strings.HasPrefix(base, "http://") {
		base = "https://" + base
	}
	base = strings.TrimSuffix(base, "/")

	return &remoteConn{
		baseURL: base,
		client: &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
			Timeout:   30 * time.Second,
		},
	}, nil
}

// Send posts an event to the enterprise daemon via the REST API.
func (c *remoteConn) Send(eventType string, payload []byte) ([]byte, error) {
	body, err := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": json.RawMessage(payload),
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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

// Health queries the enterprise daemon health endpoint.
func (c *remoteConn) Health() ([]byte, error) {
	resp, err := c.client.Get(c.baseURL + "/health")
	if err != nil {
		return nil, fmt.Errorf("health check: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *remoteConn) Close() error {
	c.client.CloseIdleConnections()
	return nil
}
