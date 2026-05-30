//go:build enterprise

package mtls

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/grimlocker/grimdb/security"
)

// Bridge wraps an HTTP server to enforce mTLS and extract client identities.
type Bridge struct {
	certMgr  *CertManager
	auditLog security.AuditLog
}

// NewBridge creates an mTLS bridge using the given CertManager.
func NewBridge(certMgr *CertManager, auditLog security.AuditLog) *Bridge {
	return &Bridge{certMgr: certMgr, auditLog: auditLog}
}

// ListenAndServeTLS starts an mTLS listener on addr and serves with handler.
func (b *Bridge) ListenAndServeTLS(addr string, handler http.Handler) error {
	tlsCfg, err := b.certMgr.TLSConfig()
	if err != nil {
		return fmt.Errorf("mTLS config: %w", err)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	tlsLn := tls.NewListener(ln, tlsCfg)
	log.Printf("[mtls] Listening on %s (mTLS required, TLS 1.3)", addr)

	srv := &http.Server{Handler: b.wrapHandler(handler)}
	return srv.Serve(tlsLn)
}

// wrapHandler injects client identity into the request context.
func (b *Bridge) wrapHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tlsState := r.TLS
		clientID := ClientIdentity(tlsState)

		if tlsState != nil && len(tlsState.PeerCertificates) == 0 {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}

		if clientID != "" && b.auditLog != nil {
			b.auditLog.Append(security.SecurityEvent{
				Level:     security.LevelInfo,
				Module:    "mtls-bridge",
				Message:   fmt.Sprintf("client connected from %s", r.RemoteAddr),
				SubjectID: clientID,
			})
		}

		ctx := context.WithValue(r.Context(), clientIdentityKey, clientID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ── Context key for client identity ──────────────────────────────────────────

type contextKey string

const clientIdentityKey contextKey = "mtls-client-identity"

// ClientIdentityFromContext retrieves the client CN from a request context.
func ClientIdentityFromContext(ctx context.Context) string {
	if v := ctx.Value(clientIdentityKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
