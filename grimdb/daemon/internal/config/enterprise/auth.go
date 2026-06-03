//go:build enterprise

package enterprise

package enterprise

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
)

// OIDCProvider implements provider.AuthProvider for the Enterprise tier.
// Authentication is performed by validating a JWT access token from an
// OIDC-compliant identity provider (Keycloak, Azure AD, Okta, etc.).
type OIDCProvider struct {
	cfg        *EnterpriseConfig
	secMod     *security.Module
	blockStore oidcBlockStore
	appDir     string

	mu         sync.Mutex
	jwks       map[string]*rsa.PublicKey // kid → public key
	jwksExpiry time.Time

	tokenCacheMu sync.RWMutex
	tokenCache   map[string]cachedToken
}

// oidcBlockStore is the subset of BlockStoreImpl needed by OIDCProvider.
type oidcBlockStore interface {
	SetMVKFunc(fn func() []byte)
	LoadIndex() error
}

type cachedToken struct {
	claims jwtClaims
	expiry time.Time
}

// NewOIDCProvider creates an OIDCProvider bound to the given security module.
func NewOIDCProvider(cfg *EnterpriseConfig, secMod *security.Module, bs oidcBlockStore) *OIDCProvider {
	return &OIDCProvider{
		cfg:        cfg,
		secMod:     secMod,
		blockStore: bs,
		appDir:     cfg.AppDir,
		jwks:       make(map[string]*rsa.PublicKey),
		tokenCache: make(map[string]cachedToken),
	}
}

// HandleUnlockEvent returns the kernel.Handler for AUTH.UNLOCK events.
// The payload JSON must contain a "token" field with the JWT access token.
func (o *OIDCProvider) HandleUnlockEvent(
	bus kernel.Dispatcher,
	sessionCtx *security.SessionContext,
	onSessionKey func(key []byte, handle string),
) kernel.Handler {
	return func(e kernel.Event) error {
		var req struct {
			Token  string `json:"token"`
			AppDir string `json:"app_dir"`
		}
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			return replyOIDCFail(bus, e, "invalid request payload")
		}
		// Backwards compat: "password" field accepted as token.
		if req.Token == "" {
			var alt struct {
				Password string `json:"password"`
			}
			_ = json.Unmarshal(e.Payload, &alt)
			req.Token = alt.Password
		}
		if req.Token == "" {
			return replyOIDCFail(bus, e, "token is required")
		}

		// Step 0: Lockdown check.
		if state := o.secMod.Lockdown().State(); state == security.LockdownHard {
			lockdownErr := gerrors.NewAuthLockdownError(0).WithModule("oidc-auth")
			log.Printf("[oidc-auth:FAIL] %s", lockdownErr.Error())
			return replyOIDCFail(bus, e, "hard lockdown active")
		}

		// Step 1: Validate JWT.
		claims, err := o.validateToken(req.Token)
		if err != nil {
			authErr := gerrors.NewAuthInvalidError("jwt_verification", err).WithModule("oidc-auth")
			log.Printf("[oidc-auth:FAIL] token validation: %s", authErr.Error())
			state, _ := o.secMod.Lockdown().RecordFailure()
			if state == security.LockdownHard {
				return replyOIDCFail(bus, e, "too many failures: hard lockdown")
			}
			return replyOIDCFail(bus, e, "invalid or expired token")
		}
		log.Printf("[oidc-auth:1/7] Token valid, subject=%s", claims.Subject)

		// Step 1b: Derive MVK from OIDC claims + server entropy.
		mvk, err := o.deriveMVKFromClaims(claims)
		if err != nil {
			derivErr := gerrors.NewCryptoKeyDerivationError("oidc_mvk_derive", err).WithModule("oidc-auth")
			log.Printf("[oidc-auth:FAIL:1b] %s", derivErr.Error())
			return replyOIDCFail(bus, e, "MVK derivation failed")
		}
		log.Printf("[oidc-auth:1/7] MVK derived (len=%d)", len(mvk))

		// Step 2: Store key in locked memory.
		handle, err := o.secMod.StoreMVK(mvk)
		for i := range mvk {
			mvk[i] = 0
		}
		if err != nil {
			// err is already *gerrors.GrimlockError (ErrCodeSecurityMemlock) from StoreMVK
			log.Printf("[oidc-auth:FAIL:2/7] StoreMVK: %v", err)
			return replyOIDCFail(bus, e, "failed to store key material")
		}
		log.Printf("[oidc-auth:2/7] MVK stored (handle=%s)", handle)

		// Step 3: Wire MVK retrieval into blockstore.
		o.blockStore.SetMVKFunc(func() []byte {
			k, _ := o.secMod.RetrieveMVK(handle)
			return k
		})

		// Step 4: Load block index.
		if err := o.blockStore.LoadIndex(); err != nil {
			log.Printf("[oidc-auth:4/7] LoadIndex error (empty index): %v", err)
		}

		// Step 5: Open STORAGE gate.
		keyReadyPayload, _ := json.Marshal(map[string]bool{"ready": true})
		_ = bus.Dispatch(kernel.NewEvent("auth", kernel.EvAuthKeyReady, keyReadyPayload))

		// Step 6: Mark session unlocked.
		sessionCtx.Unlock(handle)

		// Step 7: Record success, emit audit event, generate session key.
		o.secMod.Lockdown().RecordSuccess()
		o.secMod.Audit().Append(security.SecurityEvent{
			Level:     security.LevelInfo,
			Module:    "oidc-auth",
			Message:   "OIDC authentication successful",
			SubjectID: claims.Subject,
		})

		sessionKey := make([]byte, 32)
		if _, err := rand.Read(sessionKey); err != nil {
			log.Printf("[oidc-auth:7/7] WARN: session key generation failed: %v", err)
			sessionKey = nil
		}
		sessionKeyB64 := ""
		if sessionKey != nil {
			sessionKeyB64 = base64.StdEncoding.EncodeToString(sessionKey)
			if onSessionKey != nil {
				onSessionKey(sessionKey, "")
			}
			for i := range sessionKey {
				sessionKey[i] = 0
			}
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"mvk_handle":  handle,
			"session_key": sessionKeyB64,
			"subject_id":  claims.Subject,
		})
		return bus.Dispatch(kernel.ReplyEvent("auth", kernel.EvAuthResult, e, payload))
	}
}

func (o *OIDCProvider) StoreMVK(key []byte) (string, error) { return o.secMod.StoreMVK(key) }
func (o *OIDCProvider) RetrieveMVK(handle string) ([]byte, bool) {
	return o.secMod.RetrieveMVK(handle)
}
func (o *OIDCProvider) RevokeMVK(handle string)             { o.secMod.RevokeMVK(handle) }
func (o *OIDCProvider) Lockdown() security.LockdownManager  { return o.secMod.Lockdown() }
func (o *OIDCProvider) AuditLog() security.AuditLog         { return o.secMod.Audit() }
func (o *OIDCProvider) Tier() string                        { return "oidc-jwt" }

// ── JWT validation ────────────────────────────────────────────────────────────

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtClaims struct {
	Subject  string `json:"sub"`
	Issuer   string `json:"iss"`
	Audience any    `json:"aud"`
	Expiry   int64  `json:"exp"`
	IssuedAt int64  `json:"iat"`
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func (o *OIDCProvider) validateToken(tokenStr string) (jwtClaims, error) {
	// Check token cache (5-minute TTL).
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	o.tokenCacheMu.RLock()
	if cached, ok := o.tokenCache[tokenHash]; ok && time.Now().Before(cached.expiry) {
		o.tokenCacheMu.RUnlock()
		return cached.claims, nil
	}
	o.tokenCacheMu.RUnlock()

	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return jwtClaims{}, errors.New("malformed JWT: expected 3 parts")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtClaims{}, fmt.Errorf("decode header: %w", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return jwtClaims{}, fmt.Errorf("parse header: %w", err)
	}
	if header.Alg != "RS256" {
		return jwtClaims{}, fmt.Errorf("unsupported algorithm: %s (RS256 required)", header.Alg)
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtClaims{}, fmt.Errorf("decode claims: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return jwtClaims{}, fmt.Errorf("parse claims: %w", err)
	}

	if time.Now().Unix() > claims.Expiry {
		return jwtClaims{}, errors.New("token expired")
	}
	if claims.Issuer != o.cfg.OIDCProviderURL {
		return jwtClaims{}, fmt.Errorf("issuer mismatch: got %q want %q", claims.Issuer, o.cfg.OIDCProviderURL)
	}
	if !audienceContains(claims.Audience, o.cfg.OIDCClientID) {
		return jwtClaims{}, fmt.Errorf("audience missing client_id %q", o.cfg.OIDCClientID)
	}

	pubKey, err := o.getPublicKey(header.Kid)
	if err != nil {
		return jwtClaims{}, fmt.Errorf("get public key: %w", err)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtClaims{}, fmt.Errorf("decode signature: %w", err)
	}
	hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hashed[:], sigBytes); err != nil {
		return jwtClaims{}, fmt.Errorf("signature invalid: %w", err)
	}

	// Cache the validated token.
	o.tokenCacheMu.Lock()
	o.tokenCache[tokenHash] = cachedToken{claims: claims, expiry: time.Now().Add(5 * time.Minute)}
	o.tokenCacheMu.Unlock()

	return claims, nil
}

func audienceContains(aud any, clientID string) bool {
	switch v := aud.(type) {
	case string:
		return v == clientID
	case []interface{}:
		for _, a := range v {
			if s, ok := a.(string); ok && s == clientID {
				return true
			}
		}
	}
	return false
}

func (o *OIDCProvider) getPublicKey(kid string) (*rsa.PublicKey, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if time.Now().Before(o.jwksExpiry) {
		if key, ok := o.jwks[kid]; ok {
			return key, nil
		}
	}

	jwksURL := strings.TrimRight(o.cfg.OIDCProviderURL, "/") + "/.well-known/jwks.json"
	resp, err := http.Get(jwksURL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read JWKS: %w", err)
	}

	var jwksResp jwksResponse
	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return nil, fmt.Errorf("parse JWKS: %w", err)
	}

	o.jwks = make(map[string]*rsa.PublicKey)
	for _, k := range jwksResp.Keys {
		if k.Kty != "RSA" || k.Use != "sig" {
			continue
		}
		pubKey, err := parseJWKRSA(k)
		if err != nil {
			log.Printf("[oidc-auth] skip key %s: %v", k.Kid, err)
			continue
		}
		o.jwks[k.Kid] = pubKey
	}
	o.jwksExpiry = time.Now().Add(time.Hour)

	key, ok := o.jwks[kid]
	if !ok {
		return nil, fmt.Errorf("key ID %q not found in JWKS", kid)
	}
	return key, nil
}

func parseJWKRSA(k jwkKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("decode N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("decode E: %w", err)
	}
	n := new(big.Int).SetBytes(nBytes)
	e := int(new(big.Int).SetBytes(eBytes).Int64())
	pub := &rsa.PublicKey{N: n, E: e}
	if _, err := x509.MarshalPKIXPublicKey(pub); err != nil {
		return nil, fmt.Errorf("invalid RSA key: %w", err)
	}
	return pub, nil
}

// deriveMVKFromClaims derives a deterministic 32-byte MVK from OIDC subject +
// issuer + server entropy seed. This ties the encryption key to both the
// user identity and the server, preventing cross-server key reuse.
func (o *OIDCProvider) deriveMVKFromClaims(claims jwtClaims) ([]byte, error) {
	entropySeed, err := readEntropySeed32(o.cfg.EntropyPath)
	if err != nil {
		return nil, fmt.Errorf("entropy seed: %w", err)
	}
	h := sha256.New()
	h.Write(entropySeed)
	h.Write([]byte(claims.Subject))
	h.Write([]byte(claims.Issuer))
	var iatBuf [8]byte
	binary.BigEndian.PutUint64(iatBuf[:], uint64(claims.IssuedAt))
	h.Write(iatBuf[:])
	return h.Sum(nil), nil
}

func readEntropySeed32(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		// First run or missing entropy file: use random seed.
		seed := make([]byte, 32)
		_, randErr := rand.Read(seed)
		return seed, randErr
	}
	defer f.Close()
	seed := make([]byte, 32)
	if _, err := io.ReadFull(f, seed); err != nil {
		return nil, err
	}
	return seed, nil
}

func replyOIDCFail(bus kernel.Dispatcher, req kernel.Event, reason string) error {
	log.Printf("[oidc-auth:FAIL] reason=%q", reason)
	payload, _ := json.Marshal(map[string]interface{}{"success": false, "reason": reason})
	return bus.Dispatch(kernel.ReplyEvent("auth", kernel.EvAuthResult, req, payload))
}
