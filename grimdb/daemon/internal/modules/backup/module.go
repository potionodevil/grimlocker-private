// Package backup implements the kernel.Module owning the BACKUP channel.
//
// Two-phase import:
//
//	Phase 1 (BACKUP.PEEK):      Reads plaintext header — no key required.
//	                             Returns metadata + session_id.
//	Phase 2 (BACKUP.AUTHORIZE): Decrypts payload with MVK, imports blocks.
//	                             Verifies hardware tethering (if set).
//
// Export (BACKUP.EXPORT):
//
//	Reads all blocks, encrypts with per-export key (HKDF(MVK, ts)),
//	writes single-file blob, computes post-write SHA-256 checksum.
//
// Checksum (BACKUP.CHECKSUM):
//
//	Standalone SHA-256 of any file — no vault unlock required.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
	"github.com/grimlocker/grimdb/engine/crypto"
	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
)

const moduleID = "backup"

// GrimlockerVersion is injected by main.go at startup.
var GrimlockerVersion = "unknown"

// ExportPolicyFn is called before an export. nil = allow all (single-user).
// Enterprise tier can inject RBAC checks here.
type ExportPolicyFn func(origin string) error

// KeyResolver resolves an MVK handle to raw key bytes.
type KeyResolver func(handle string) ([]byte, bool)

// ArgonSaltResolver returns the current vault ArgonSalt.
type ArgonSaltResolver func() ([]byte, error)

// Module is the kernel.Module handling all BACKUP.* events.
type Module struct {
	crypto       crypto.Provider
	keyResolver  KeyResolver
	saltResolver ArgonSaltResolver
	store        storage.BlockStore
	policy       ExportPolicyFn
	dispatcher   kernel.Dispatcher
	registry     *HandlerRegistry
	sessions     *SessionStore
	stopGC       chan struct{}
}

// NewModule creates the backup module.
func NewModule(
	cryptoP  crypto.Provider,
	keys     KeyResolver,
	saltFn   ArgonSaltResolver,
	store    storage.BlockStore,
	policy   ExportPolicyFn,
) *Module {
	return &Module{
		crypto:       cryptoP,
		keyResolver:  keys,
		saltResolver: saltFn,
		store:        store,
		policy:       policy,
		sessions:     newSessionStore(),
		stopGC:       make(chan struct{}),
	}
}

func (m *Module) ID() string         { return moduleID }
func (m *Module) Channels() []string { return []string{"BACKUP"} }

func (m *Module) Start(ctx context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	m.registry = m.buildRegistry()

	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.sessions.pruneExpired()
			case <-m.stopGC:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (m *Module) Stop() error {
	close(m.stopGC)
	return nil
}

func (m *Module) Handle(e kernel.Event) error {
	err := m.registry.Dispatch(e)
	if err != nil {
		log.Printf("[backup] handler error event=%s: %v", e.Type, err)
	}
	return err
}

func (m *Module) buildRegistry() *HandlerRegistry {
	r := NewHandlerRegistry()

	r.MustRegister(kernel.EvBackupExport,
		JSONSchemaValidator(func(p *engbackup.ExportRequest) error {
			if p.DestPath == "" {
				return fmt.Errorf("export: dest_path is required")
			}
			return nil
		}),
		m.handleExport,
	)

	r.MustRegister(kernel.EvBackupPeek,
		JSONSchemaValidator(func(p *engbackup.PeekRequest) error {
			if p.SourcePath == "" {
				return fmt.Errorf("peek: source_path is required")
			}
			return nil
		}),
		m.handlePeek,
	)

	r.MustRegister(kernel.EvBackupAuthorize,
		JSONSchemaValidator(func(p *engbackup.AuthorizeRequest) error {
			if p.SessionID == "" {
				return fmt.Errorf("authorize: session_id is required")
			}
			if p.KeyHandle == "" {
				return fmt.Errorf("authorize: key_handle is required")
			}
			return nil
		}),
		m.handleAuthorize,
	)

	r.MustRegister(kernel.EvBackupChecksum,
		JSONSchemaValidator(func(p *engbackup.ChecksumRequest) error {
			if p.Path == "" {
				return fmt.Errorf("checksum: path is required")
			}
			return nil
		}),
		m.handleChecksum,
	)

	return r
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (m *Module) handleExport(e kernel.Event) error {
	var req engbackup.ExportRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyExportError(e, gerrors.NewProtocolError("export_unmarshal", err))
	}

	if m.policy != nil {
		if err := m.policy(e.Origin); err != nil {
			return m.replyExportError(e, gerrors.Wrap(gerrors.ErrCodeSecurityUnauthorized, "export denied by policy", err))
		}
	}

	mvk, argonSalt, err := m.resolveMVKAndSalt(req.DestPath, "")
	if err != nil {
		return m.replyExportError(e, err)
	}

	sha256hex, count, err := buildBlob(m.crypto, m.store, mvk, argonSalt, req.DestPath, req.HardwareTether, GrimlockerVersion)
	if err != nil {
		return m.replyExportError(e, gerrors.Wrap(gerrors.ErrCodeBackupChecksumFailed, "export failed", err))
	}

	completePayload, _ := json.Marshal(engbackup.ChecksumCompleteEvent{
		Path:            req.DestPath,
		SHA256:          sha256hex,
		ExportTimestamp: time.Now().Unix(),
	})
	_ = m.dispatcher.Dispatch(kernel.NewEvent(moduleID, kernel.EvBackupChecksumComplete, completePayload))

	m.audit(fmt.Sprintf("vault exported to %s (%d entries)", req.DestPath, count))

	res, _ := json.Marshal(engbackup.ExportResult{Path: req.DestPath, SHA256: sha256hex, EntryCount: count})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

func (m *Module) handlePeek(e kernel.Event) error {
	var req engbackup.PeekRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyPeekError(e, gerrors.NewProtocolError("peek_unmarshal", err))
	}

	peek, err := peekBlob(m.sessions, req.SourcePath)
	if err != nil {
		return m.replyPeekError(e, gerrors.NewBackupInvalidMagicError(req.SourcePath))
	}

	res, _ := json.Marshal(peek)
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

func (m *Module) handleAuthorize(e kernel.Event) error {
	var req engbackup.AuthorizeRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyAuthorizeError(e, gerrors.NewProtocolError("authorize_unmarshal", err))
	}

	mvk, argonSalt, err := m.resolveMVKAndSalt("", req.KeyHandle)
	if err != nil {
		return m.replyAuthorizeError(e, err)
	}

	imported, skipped, err := authorizeImport(m.sessions, m.crypto, m.store, req.SessionID, mvk, argonSalt, req.Merge)
	if err != nil {
		code := 0
		if ge, ok := err.(*gerrors.GrimlockError); ok {
			code = ge.Code
		}
		res, _ := json.Marshal(engbackup.AuthorizeResult{Error: err.Error(), ErrorCode: code})
		return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
	}

	m.audit(fmt.Sprintf("vault imported from air-gap backup (imported=%d, skipped=%d)", imported, skipped))

	res, _ := json.Marshal(engbackup.AuthorizeResult{ImportedCount: imported, SkippedCount: skipped})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

func (m *Module) handleChecksum(e kernel.Event) error {
	var req engbackup.ChecksumRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyChecksumError(e, gerrors.NewProtocolError("checksum_unmarshal", err))
	}

	sum, err := checksumFile(req.Path)
	if err != nil {
		return m.replyChecksumError(e, gerrors.Wrap(gerrors.ErrCodeStorageIO, "checksum read failed", err))
	}

	res, _ := json.Marshal(engbackup.ChecksumResult{Path: req.Path, SHA256: sum})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func (m *Module) resolveMVKAndSalt(_, keyHandle string) (mvk, argonSalt []byte, err error) {
	if keyHandle != "" {
		var ok bool
		mvk, ok = m.keyResolver(keyHandle)
		if !ok {
			return nil, nil, gerrors.NewCryptoHandleUnknownError(keyHandle)
		}
	}
	argonSalt, err = m.saltResolver()
	if err != nil {
		return nil, nil, gerrors.NewSecurityMVKMissingError("backup")
	}
	return mvk, argonSalt, nil
}

func (m *Module) audit(msg string) {
	payload, _ := json.Marshal(map[string]string{"module": moduleID, "message": msg, "level": "info"})
	_ = m.dispatcher.Dispatch(kernel.NewEvent(moduleID, kernel.EvSecAudit, payload))
}

func (m *Module) replyExportError(e kernel.Event, err error) error {
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	res, _ := json.Marshal(engbackup.ExportResult{Error: err.Error(), ErrorCode: code})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

func (m *Module) replyPeekError(e kernel.Event, err error) error {
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	res, _ := json.Marshal(engbackup.PeekResult{Error: err.Error(), ErrorCode: code})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

func (m *Module) replyAuthorizeError(e kernel.Event, err error) error {
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	res, _ := json.Marshal(engbackup.AuthorizeResult{Error: err.Error(), ErrorCode: code})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}

func (m *Module) replyChecksumError(e kernel.Event, err error) error {
	code := 0
	if ge, ok := err.(*gerrors.GrimlockError); ok {
		code = ge.Code
	}
	res, _ := json.Marshal(engbackup.ChecksumResult{Error: err.Error(), ErrorCode: code})
	return m.dispatcher.Dispatch(kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res))
}
