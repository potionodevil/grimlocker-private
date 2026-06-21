// Package backup implementiert das kernel.Module, das den BACKUP-Channel besitzt.
//
// Zwei-Phasen-Import:
//
//	Phase 1 (BACKUP.PEEK):      Liest Plaintext-Header — kein Key nötig.
//	                             Gibt Metadaten + session_id zurück.
//	Phase 2 (BACKUP.AUTHORIZE): Entschlüsselt Payload mit MVK, importiert Blocks.
//	                             Prüft Hardware-Tethering (wenn gesetzt).
//
// Export (BACKUP.EXPORT):
//
//	Liest alle Blocks aus dem Store, verschlüsselt sie mit per-Export-Key (HKDF(MVK, ts)),
//	schreibt Single-File-Blob, berechnet Post-Write-SHA256-Checksum.
//
// Checksum (BACKUP.CHECKSUM):
//
//	Standalone SHA-256 einer bestehenden Datei — kein Vault-Unlock nötig.
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

// GrimlockerVersion wird beim Bauen injiziert (via main.go oder Linker-Flag).
// Fallback auf "unknown" wenn nicht gesetzt.
var GrimlockerVersion = "unknown"

// ExportPolicyFn wird vor einem Export aufgerufen. nil = erlaubt (Single-User).
// Enterprise-Tier kann RBAC-Checks hier einhängen.
type ExportPolicyFn func(origin string) error

// KeyResolver löst einen MVK-Handle zu den rohen Key-Bytes auf.
// Wird vom Security-Modul bereitgestellt.
type KeyResolver func(handle string) ([]byte, bool)

// ArgonSaltResolver gibt den aktuellen ArgonSalt der Vault zurück.
// Wird für Tethering und Import benötigt.
type ArgonSaltResolver func() ([]byte, error)

// Module ist das kernel.Module, das alle BACKUP.*-Events verarbeitet.
type Module struct {
	crypto      crypto.Provider
	keyResolver KeyResolver
	saltResolver ArgonSaltResolver
	store       storage.BlockStore
	policy      ExportPolicyFn
	dispatcher  kernel.Dispatcher
	registry    *HandlerRegistry
	sessions    *SessionStore
	stopGC      chan struct{}
}

// NewModule erstellt das Backup-Modul.
//   - cryptoP:     Crypto-Provider (ChaCha20, HKDF)
//   - keys:        MVK-Handle-Resolver vom Security-Modul
//   - saltFn:      ArgonSalt-Resolver der Vault
//   - store:       BlockStore für Bulk-Read/Write beim Export/Import
//   - policy:      optional; nil = alle Exports erlaubt
func NewModule(
	cryptoP    crypto.Provider,
	keys       KeyResolver,
	saltFn     ArgonSaltResolver,
	store      storage.BlockStore,
	policy     ExportPolicyFn,
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

	// Session-GC: bereinigt abgelaufene Sessions alle 60 Sekunden
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

// ─── Handler ──────────────────────────────────────────────────────────────────

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

	mvk, argonSalt, err := m.resolveMVKAndSalt("export", "")
	if err != nil {
		return m.replyExportError(e, err)
	}

	sha256hex, count, err := buildBlob(m.crypto, m.store, mvk, argonSalt, req.DestPath, req.HardwareTether, GrimlockerVersion)
	if err != nil {
		return m.replyExportError(e, gerrors.Wrap(gerrors.ErrCodeBackupChecksumFailed, "export failed", err))
	}

	// BACKUP.CHECKSUM_COMPLETE emittieren
	completePayload, _ := json.Marshal(engbackup.ChecksumCompleteEvent{
		Path:            req.DestPath,
		SHA256:          sha256hex,
		ExportTimestamp: time.Now().Unix(),
	})
	_ = m.dispatcher.Dispatch(kernel.NewEvent(moduleID, kernel.EvBackupChecksumComplete, completePayload))

	// Audit-Log
	m.audit(fmt.Sprintf("vault exported to %s (%d entries)", req.DestPath, count))

	res, _ := json.Marshal(engbackup.ExportResult{
		Path:       req.DestPath,
		SHA256:     sha256hex,
		EntryCount: count,
	})
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

	mvk, argonSalt, err := m.resolveMVKAndSalt("authorize", req.KeyHandle)
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
		reply := kernel.ReplyEvent(moduleID, kernel.EvBackupResult, e, res)
		return m.dispatcher.Dispatch(reply)
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

// ─── Hilfsfunktionen ──────────────────────────────────────────────────────────

// resolveMVKAndSalt löst MVK (über keyHandle oder saltResolver) und ArgonSalt auf.
// Bei leerem keyHandle wird nur der ArgonSalt gebraucht (z.B. für Peek — aber Peek
// ruft diese Funktion gar nicht auf). Für Export/Authorize wird keyHandle benötigt.
func (m *Module) resolveMVKAndSalt(op, keyHandle string) (mvk, argonSalt []byte, err error) {
	if keyHandle != "" {
		var ok bool
		mvk, ok = m.keyResolver(keyHandle)
		if !ok {
			return nil, nil, gerrors.NewCryptoHandleUnknownError(keyHandle)
		}
	}

	argonSalt, err = m.saltResolver()
	if err != nil {
		return nil, nil, gerrors.NewSecurityMVKMissingError(op)
	}
	return mvk, argonSalt, nil
}

func (m *Module) audit(msg string) {
	payload, _ := json.Marshal(map[string]string{
		"module":  moduleID,
		"message": msg,
		"level":   "info",
	})
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
