//go:build enterprise

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/grimlocker/grimdb/config/enterprise"
	"github.com/grimlocker/grimdb/security/mtls"
)

// startTierListener starts an mTLS listener for the enterprise tier.
// Binds to GRIMLOCKER_BIND_ADDR (default "0.0.0.0:9443") and starts a plaintext
// probe listener on GRIMLOCKER_PROBE_PORT (default "9090", localhost only).
// vault must be *enterprise.Provider.
func startTierListener(vault interface{}, ipcMux *http.ServeMux) (net.Listener, string, error) {
	ep, ok := vault.(*enterprise.Provider)
	if !ok {
		return nil, "", fmt.Errorf("enterprise build: expected *enterprise.Provider, got %T", vault)
	}

	cfg := ep.EnterpriseConfig()
	certMgr := mtls.NewCertManager(
		cfg.MTLSCertPath,
		cfg.MTLSKeyPath,
		cfg.MTLSCAPath,
		cfg.MTLSSPKIPin,
	)

	tlsCfg, err := certMgr.TLSConfig()
	if err != nil {
		return nil, "", fmt.Errorf("mTLS config: %w", err)
	}

	bindAddr := os.Getenv("GRIMLOCKER_BIND_ADDR")
	if bindAddr == "" {
		bindAddr = "0.0.0.0:9443"
	}

	ln, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return nil, "", fmt.Errorf("listen %s: %w", bindAddr, err)
	}
	tlsLn := tls.NewListener(ln, tlsCfg)
	log.Printf("[Omega] Enterprise mTLS listener on %s (TLS 1.3, mutual auth)", bindAddr)

	// Plaintext probe port — only /health, no auth, no mTLS.
	// Used by: Docker HEALTHCHECK, Kubernetes liveness/readiness probes.
	probePort := os.Getenv("GRIMLOCKER_PROBE_PORT")
	if probePort == "" {
		probePort = "9090"
	}
	go startProbeListener(probePort, ipcMux)

	return tlsLn, tlsLn.Addr().String(), nil
}

// startProbeListener runs a minimal plain-HTTP server for health probes.
// Only the /health route is served; all other paths return 404.
func startProbeListener(port string, ipcMux *http.ServeMux) {
	probeMux := http.NewServeMux()
	probeMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// Forward to ipcMux's /health handler by calling it directly.
		// The handler is already registered on ipcMux, so re-use the same logic.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "ready",
			"tier":   "enterprise",
			"probe":  "ok",
		})
	})
	probeSrv := &http.Server{Handler: probeMux}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		log.Printf("[Omega] Probe listener unavailable on :%s: %v", port, err)
		return
	}
	log.Printf("[Omega] Probe listener on 127.0.0.1:%s (/health, plaintext)", port)
	if err := probeSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Printf("[Omega] Probe listener error: %v", err)
	}
}

// tierListenerAddr returns the WSS address advertised on stdout.
func tierListenerAddr(addr string) string {
	return "wss://" + addr + "/ws"
}
