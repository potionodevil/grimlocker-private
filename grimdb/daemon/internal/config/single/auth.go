//go:build !enterprise

// Package single provides the Single-User tier implementation of provider interfaces.
// LocalAuth encapsulates the full vault-unlock flow (Steps 0–7) that was
// previously inlined as makeAuthUnlockHandler in cmd/daemon/main.go.
package single

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"runtime/debug"

	rustbridge "github.com/grimlocker/grimdb/daemon/internal/bridge"
	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	secmod "github.com/grimlocker/grimdb/daemon/internal/modules/security"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"
)

// LocalAuth implements provider.AuthProvider for Single-User tier.
// Authentication is done via Argon2id password derivation stored in locked memory.
type LocalAuth struct {
	secMod     *secmod.Module
	blockStore *grimdb.BlockStoreImpl
	appDir     string
}

// NewLocalAuth creates a LocalAuth bound to the given security module and block store.
func NewLocalAuth(secMod *secmod.Module, blockStore *grimdb.BlockStoreImpl, appDir string) *LocalAuth {
	return &LocalAuth{
		secMod:     secMod,
		blockStore: blockStore,
		appDir:     appDir,
	}
}

// HandleUnlockEvent returns the kernel.Handler for AUTH.UNLOCK events.
// This is the full 7-step unlock flow extracted from makeAuthUnlockHandler.
func (a *LocalAuth) HandleUnlockEvent(
	bus kernel.Dispatcher,
	sessionCtx *security.SessionContext,
	onSessionKey func(key []byte, handle string),
) kernel.Handler {
	return func(e kernel.Event) (err error) {
		// Catch ANY panic in this handler — prevents silent goroutine death.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[auth] PANIC in unlock handler: %v\n%s", r, debug.Stack())
				panic(r) // re-panic — the bus wrapper also recovers
			}
		}()

		var req struct {
			Password string `json:"password"`
			AppDir   string `json:"app_dir"`
		}
		if err := json.Unmarshal(e.Payload, &req); err != nil {
			log.Printf("[unlock:FAIL] payload unmarshal: %v", err)
			return err
		}

		dir := req.AppDir
		if dir == "" {
			dir = a.appDir
		}

		// Step 0/7 — Lockdown check.
		if state := a.secMod.Lockdown().State(); state == security.LockdownHard {
			lockdownErr := gerrors.NewAuthLockdownError(0)
			log.Printf("[unlock:FAIL] hard lockdown active — %s", lockdownErr.Error())
			return replyAuthFail(bus, e, "hard lockdown active")
		}

		// Step 1/7 — Derive & verify MVK.
		log.Printf("[auth] AUTH.UNLOCK received — attempting vault unlock")
		log.Printf("[unlock:1/7] UnlockVault starting")
		mvk, err := grimdb.UnlockVault(req.Password, dir)
		if err != nil {
			authErr := gerrors.NewAuthInvalidError("password_hash", err)
			log.Printf("[unlock:FAIL:1/7] UnlockVault: %s", authErr.Error())
			state, _ := a.secMod.Lockdown().RecordFailure()
			if state == security.LockdownHard {
				lockdownErr := gerrors.NewAuthLockdownError(0)
				log.Printf("[unlock:FAIL:1/7] lockdown triggered: %s", lockdownErr.Error())
				return replyAuthFailLockdown(bus, e, "too many failures: hard lockdown", 0, 0, true)
			}
			remaining := a.secMod.Lockdown().RemainingAttempts()
			until := a.secMod.Lockdown().LockdownUntil().Unix()
			log.Printf("[unlock:FAIL:1/7] %s", gerrors.NewAuthLockdownError(remaining).Error())
			return replyAuthFailLockdown(bus, e, "invalid password", remaining, until, false)
		}
		log.Printf("[unlock:1/7] UnlockVault OK (key len=%d)", len(mvk))

		// Step 2/7 — Store key in locked memory.
		log.Printf("[unlock:2/7] StoreMVK starting")
		handle, err := a.secMod.StoreMVK(mvk)
		for i := range mvk {
			mvk[i] = 0
		}
		if err != nil {
			// err is already a *gerrors.GrimlockError (ErrCodeSecurityMemlock) from StoreMVK
			log.Printf("[unlock:FAIL:2/7] StoreMVK: %v", err)
			return replyAuthFail(bus, e, "failed to store key material")
		}
		log.Printf("[unlock:2/7] StoreMVK OK (handle=<redacted>)")

		// Step 3/7 — Wire MVK into blockstore.
		log.Printf("[unlock:3/7] SetMVKFunc starting")
		a.blockStore.SetMVKFunc(func() []byte {
			k, _ := a.secMod.RetrieveMVK(handle)
			return k
		})
		log.Printf("[unlock:3/7] SetMVKFunc OK")

		// Step 4/7 — Load block index.
		log.Printf("[unlock:4/7] LoadIndex starting")
		if err := a.blockStore.LoadIndex(); err != nil {
			log.Printf("[unlock:4/7] LoadIndex error (treating as empty index): %v", err)
		}
		log.Printf("[unlock:4/7] LoadIndex OK")

		// Step 5/7 — Open STORAGE gate.
		log.Printf("[unlock:5/7] KEY_READY dispatch")
		keyReadyPayload, _ := json.Marshal(map[string]bool{"ready": true})
		_ = bus.Dispatch(kernel.NewEvent("auth", kernel.EvAuthKeyReady, keyReadyPayload))
		log.Printf("[unlock:5/7] KEY_READY dispatched")

		// Step 6/7 — Mark session as unlocked.
		log.Printf("[unlock:6/7] sessionCtx.Unlock starting")
		sessionCtx.Unlock(handle)
		log.Printf("[unlock:6/7] sessionCtx.Unlock OK")

		// Step 7/7 — Record success & generate session key.
		a.secMod.Lockdown().RecordSuccess()
		log.Printf("[unlock:7/7] RecordSuccess + AUTH.RESULT (success=true)")

		// Generate a per-session ChaCha20-Poly1305 key for SKE encryption.
		// Try the Rust enclave first; fall back to Go CSPRNG if unavailable.
		var sessionKey []byte
		var sessionKeyHandle string
		skh, ska, rustErr := rustbridge.SessionCreate()
		if rustErr != nil {
			log.Printf("[unlock:7/7] Rust session create failed, using Go fallback: %v", rustErr)
			sessionKey = make([]byte, 32)
			if _, randErr := rand.Read(sessionKey); randErr != nil {
				log.Printf("[unlock:7/7] WARN: session key generation failed: %v", randErr)
				sessionKey = nil
			}
		} else {
			sessionKey = ska[:]
			sessionKeyHandle = skh
			log.Printf("[unlock:7/7] Session key created via Rust enclave (handle=<redacted>)")
		}

		// Encode session key for the response BEFORE zeroing the local copy.
		sessionKeyB64 := ""
		if sessionKey != nil {
			sessionKeyB64 = base64.StdEncoding.EncodeToString(sessionKey)
		}

		// Inject session key + handle into the translator.
		if onSessionKey != nil && sessionKey != nil {
			onSessionKey(sessionKey, sessionKeyHandle)
		}

		// Zero the local session key copy.
		for i := range sessionKey {
			sessionKey[i] = 0
		}

		payload, _ := json.Marshal(map[string]interface{}{
			"success":     true,
			"mvk_handle":  handle,
			"session_key": sessionKeyB64,
		})
		reply := kernel.ReplyEvent("auth", kernel.EvAuthResult, e, payload)
		return bus.Dispatch(reply)
	}
}

// StoreMVK delegates to the underlying security module.
func (a *LocalAuth) StoreMVK(key []byte) (string, error) { return a.secMod.StoreMVK(key) }

// RetrieveMVK delegates to the underlying security module.
func (a *LocalAuth) RetrieveMVK(handle string) ([]byte, bool) {
	return a.secMod.RetrieveMVK(handle)
}

// RevokeMVK delegates to the underlying security module.
func (a *LocalAuth) RevokeMVK(handle string) { a.secMod.RevokeMVK(handle) }

// Lockdown returns the LockdownManager from the security module.
func (a *LocalAuth) Lockdown() security.LockdownManager { return a.secMod.Lockdown() }

// AuditLog returns the AuditLog from the security module.
func (a *LocalAuth) AuditLog() security.AuditLog { return a.secMod.Audit() }

// Tier identifies the authentication mechanism used by this provider.
func (a *LocalAuth) Tier() string { return "local-argon2id" }

// replyAuthFail emits an AUTH.RESULT failure event.
func replyAuthFail(bus kernel.Dispatcher, req kernel.Event, reason string) error {
	return replyAuthFailLockdown(bus, req, reason, 0, 0, false)
}

// replyAuthFailLockdown emits AUTH.RESULT failure with lockdown metadata.
func replyAuthFailLockdown(bus kernel.Dispatcher, req kernel.Event, reason string, remaining int, lockdownUntil int64, hardLockdown bool) error {
	log.Printf("[auth:FAIL] reason=%q remaining=%d lockdown_until=%d hard=%v", reason, remaining, lockdownUntil, hardLockdown)
	payload, _ := json.Marshal(map[string]interface{}{
		"success":        false,
		"reason":         reason,
		"remaining":      remaining,
		"lockdown_until": lockdownUntil,
		"hard_lockdown":  hardLockdown,
	})
	reply := kernel.ReplyEvent("auth", kernel.EvAuthResult, req, payload)
	return bus.Dispatch(reply)
}
