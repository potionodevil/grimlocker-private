// Package crypto implements the kernel.Module that owns the CRYPTO channel.
// All cryptographic operations (encrypt, decrypt, key-derive) are dispatched
// as events and handled here; no other module performs raw crypto.
//
// Design principles:
//
//   - Stateless with respect to key material: the Module holds no keys.
//     Every handler fetches the raw key bytes from security.Module via
//     the injected KeyResolver function, uses them for one operation,
//     and discards the reference. Keys never leave locked memory.
//
//   - Validated inputs: the HandlerRegistry enforces a PayloadValidator
//     per event type before the handler runs, so handlers can assume
//     well-formed payloads (non-empty ciphertext, required fields set).
//
//   - Typed errors: all failures return *errors.GrimlockError with a
//     code in the 3000–3999 range and are also propagated back to the
//     caller as CRYPTO.RESULT{error, error_code}.
//
// Supported events: CRYPTO.ENCRYPT, CRYPTO.DECRYPT, CRYPTO.DERIVE_KEY.
// Result event: CRYPTO.RESULT (always dispatched, even on failure).
package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	gerrors "github.com/grimlocker/grimdb/errors"
	"github.com/grimlocker/grimdb/kernel"
)

const moduleID = "crypto"

// encryptPayload is the JSON schema for CRYPTO.ENCRYPT events.
type encryptPayload struct {
	KeyHandle string `json:"key_handle"`
	Plaintext []byte `json:"plaintext"`
	AAD       []byte `json:"aad,omitempty"`
}

// decryptPayload is the JSON schema for CRYPTO.DECRYPT events.
type decryptPayload struct {
	KeyHandle  string `json:"key_handle"`
	Ciphertext []byte `json:"ciphertext"`
	Nonce      []byte `json:"nonce"`
	AAD        []byte `json:"aad,omitempty"`
}

// derivePayload is the JSON schema for CRYPTO.DERIVE_KEY events.
type derivePayload struct {
	Password []byte     `json:"password"`
	Salt     []byte     `json:"salt"`
	Opts     KDFOptions `json:"opts"`
}

// cryptoResult is the JSON schema for CRYPTO.RESULT events.
type cryptoResult struct {
	Data      []byte `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorCode int    `json:"error_code,omitempty"` // GrimlockError code for structured handling
}

// KeyResolver is called by the CryptoModule to retrieve raw key bytes from a
// handle. It is provided by the security.Module to keep key material isolated.
type KeyResolver func(handle string) ([]byte, bool)

// eventHandlerFn is the internal handler function type used by the registry.
type eventHandlerFn func(kernel.Event) error

// Module is the kernel.Module that handles all CRYPTO.* events.
// It holds no key material itself — keys are fetched via KeyResolver per event.
type Module struct {
	provider    Provider
	keyResolver KeyResolver
	dispatcher  kernel.Dispatcher
	registry    *HandlerRegistry // replaces raw map — adds payload validation + duplicate detection
}

// NewModule creates the crypto module.
func NewModule(p Provider, kr KeyResolver) *Module {
	return &Module{provider: p, keyResolver: kr}
}

func (m *Module) ID() string         { return moduleID }
func (m *Module) Channels() []string { return []string{"CRYPTO"} }

func (m *Module) Start(ctx context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	m.registry = m.buildRegistry()
	return nil
}

func (m *Module) Stop() error { return nil }

// buildRegistry returns the HandlerRegistry for all CRYPTO.* events.
// Each handler is paired with a payload validator that pre-checks the JSON schema.
// Adding a new handler requires only a new MustRegister call — no switch editing.
func (m *Module) buildRegistry() *HandlerRegistry {
	r := NewHandlerRegistry()

	// Validators enforce minimum required fields before the handler runs.
	r.MustRegister(kernel.EvCryptoEncrypt,
		JSONSchemaValidator(func(p *encryptPayload) error {
			if p.KeyHandle == "" {
				return fmt.Errorf("encrypt: key_handle is required")
			}
			if len(p.Plaintext) == 0 {
				return fmt.Errorf("encrypt: plaintext is empty")
			}
			return nil
		}),
		m.handleEncrypt,
	)

	r.MustRegister(kernel.EvCryptoDecrypt,
		JSONSchemaValidator(func(p *decryptPayload) error {
			if p.KeyHandle == "" {
				return fmt.Errorf("decrypt: key_handle is required")
			}
			if len(p.Ciphertext) == 0 {
				return fmt.Errorf("decrypt: ciphertext is empty")
			}
			if len(p.Nonce) == 0 {
				return fmt.Errorf("decrypt: nonce is required")
			}
			return nil
		}),
		m.handleDecrypt,
	)

	r.MustRegister(kernel.EvCryptoDerive,
		JSONSchemaValidator(func(p *derivePayload) error {
			if len(p.Password) == 0 {
				return fmt.Errorf("derive: password is required")
			}
			if len(p.Salt) == 0 {
				return fmt.Errorf("derive: salt is required")
			}
			return nil
		}),
		m.handleDerive,
	)

	return r
}

// Handle dispatches the event to the registered handler via the registry.
// Unknown events are silently ignored (no noisy debug logs for cross-channel events).
func (m *Module) Handle(e kernel.Event) error {
	err := m.registry.Dispatch(e)
	if err != nil {
		log.Printf("[crypto] handler error event=%s: %v", e.Type, err)
	}
	return err
}

// Ensure fmt is used (replyError still needs it).
var _ = fmt.Errorf

func (m *Module) handleEncrypt(e kernel.Event) error {
	var p encryptPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return m.replyError(e, gerrors.NewProtocolError("encrypt_unmarshal", err))
	}

	key, ok := m.keyResolver(p.KeyHandle)
	if !ok {
		return m.replyError(e, gerrors.NewCryptoHandleUnknownError(p.KeyHandle))
	}

	nonce, err := m.provider.NewNonce()
	if err != nil {
		return m.replyError(e, gerrors.NewCryptoEncryptionError("new_nonce", err))
	}

	ct, err := m.provider.Encrypt(key, nonce[:], p.Plaintext, p.AAD)
	if err != nil {
		return m.replyError(e, gerrors.NewCryptoEncryptionError("chacha20poly1305_seal", err))
	}

	// Prepend nonce to ciphertext so the caller gets a self-contained blob.
	blob := append(nonce[:], ct...)
	return m.replyOK(e, blob)
}

func (m *Module) handleDecrypt(e kernel.Event) error {
	var p decryptPayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return m.replyError(e, gerrors.NewProtocolError("decrypt_unmarshal", err))
	}

	key, ok := m.keyResolver(p.KeyHandle)
	if !ok {
		return m.replyError(e, gerrors.NewCryptoHandleUnknownError(p.KeyHandle))
	}

	pt, err := m.provider.Decrypt(key, p.Nonce, p.Ciphertext, p.AAD)
	if err != nil {
		return m.replyError(e, gerrors.NewCryptoDecryptionError("", err))
	}

	return m.replyOK(e, pt)
}

func (m *Module) handleDerive(e kernel.Event) error {
	var p derivePayload
	if err := json.Unmarshal(e.Payload, &p); err != nil {
		return m.replyError(e, gerrors.NewProtocolError("derive_unmarshal", err))
	}

	if p.Opts.KeyLen == 0 {
		p.Opts.KeyLen = 32
	}

	key, err := m.provider.DeriveArgon2id(p.Password, p.Opts)
	if err != nil {
		return m.replyError(e, gerrors.NewCryptoKeyDerivationError("argon2id", err))
	}

	return m.replyOK(e, key)
}

func (m *Module) replyOK(req kernel.Event, data []byte) error {
	res, _ := json.Marshal(cryptoResult{Data: data})
	reply := kernel.ReplyEvent(moduleID, kernel.EvCryptoResult, req, res)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) replyError(req kernel.Event, err error) error {
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	res, _ := json.Marshal(cryptoResult{Error: err.Error(), ErrorCode: code})
	reply := kernel.ReplyEvent(moduleID, kernel.EvCryptoResult, req, res)
	if dErr := m.dispatcher.Dispatch(reply); dErr != nil {
		return fmt.Errorf("%w (reply dispatch failed: %v)", err, dErr)
	}
	return err
}
