// Package shamir implementiert das SHAMIR-Kanal-Module des Grimlocker-Daemons.
//
// Dieses Module erlaubt es, den Backup-Encryption-Key (MVK-abgeleitet) in N Shares
// aufzuteilen und aus mindestens K Shares wieder herzustellen. Für Enterprise-Szenarien,
// wo kein einzelner Mitarbeiter das Backup alleine einspielen kann.
//
// Unterstützte Events:
//
//	SHAMIR.SPLIT   → SplitRequest{secret_hex, n, k} → ShamirResult{shares:[{x,y_hex}]}
//	SHAMIR.COMBINE → CombineRequest{shares:[{x,y_hex}]}   → ShamirResult{secret_hex}
package shamir

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
	"github.com/grimlocker/grimdb/engine/kernel"
)

// Module implementiert das SHAMIR-Kanal-Module.
type Module struct {
	dispatcher kernel.Dispatcher
}

// NewModule erzeugt ein neues Shamir-Module.
func NewModule() *Module { return &Module{} }

func (m *Module) ID() string            { return "shamir" }
func (m *Module) Channels() []string    { return []string{"SHAMIR"} }

func (m *Module) Start(_ context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	log.Println("[shamir] module started")
	return nil
}

func (m *Module) Stop() error {
	log.Println("[shamir] module stopped")
	return nil
}

func (m *Module) Handle(e kernel.Event) error {
	switch e.Type {
	case kernel.EvShamirSplit:
		return m.handleSplit(e)
	case kernel.EvShamirCombine:
		return m.handleCombine(e)
	default:
		return nil
	}
}

// ─── Split ────────────────────────────────────────────────────────────────────

type splitRequest struct {
	SecretHex string `json:"secret_hex"` // hex-kodiertes Secret (z.B. 32-Byte Backup-Key)
	N         int    `json:"n"`           // Gesamtzahl Shares
	K         int    `json:"k"`           // Mindestzahl für Rekonstruktion
}

type shareJSON struct {
	X    byte   `json:"x"`     // Share-Index (1..N)
	YHex string `json:"y_hex"` // hex-kodierter Share-Wert
}

type splitResult struct {
	Shares []shareJSON `json:"shares"`
	N      int         `json:"n"`
	K      int         `json:"k"`
}

func (m *Module) handleSplit(e kernel.Event) error {
	var req splitRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyError(e, fmt.Errorf("shamir.split: unmarshal: %w", err))
	}

	secret, err := hex.DecodeString(req.SecretHex)
	if err != nil {
		return m.replyError(e, fmt.Errorf("shamir.split: decode secret: %w", err))
	}

	shares, err := engbackup.SplitSecret(secret, req.N, req.K)
	if err != nil {
		return m.replyError(e, fmt.Errorf("shamir.split: %w", err))
	}

	result := splitResult{N: req.N, K: req.K, Shares: make([]shareJSON, len(shares))}
	for i, sh := range shares {
		result.Shares[i] = shareJSON{X: sh.X, YHex: hex.EncodeToString(sh.Y)}
	}

	payload, _ := json.Marshal(result)
	m.dispatcher.Dispatch(kernel.ReplyEvent("shamir", kernel.EvShamirResult, e, payload)) //nolint:errcheck
	log.Printf("[shamir:SPLIT] n=%d k=%d secretLen=%d", req.N, req.K, len(secret))
	return nil
}

// ─── Combine ──────────────────────────────────────────────────────────────────

type combineRequest struct {
	Shares []shareJSON `json:"shares"`
}

type combineResult struct {
	SecretHex string `json:"secret_hex"`
}

func (m *Module) handleCombine(e kernel.Event) error {
	var req combineRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyError(e, fmt.Errorf("shamir.combine: unmarshal: %w", err))
	}

	shares := make([]engbackup.ShamirShare, len(req.Shares))
	for i, sh := range req.Shares {
		y, err := hex.DecodeString(sh.YHex)
		if err != nil {
			return m.replyError(e, fmt.Errorf("shamir.combine: decode share[%d]: %w", i, err))
		}
		shares[i] = engbackup.ShamirShare{X: sh.X, Y: y}
	}

	secret, err := engbackup.CombineShares(shares)
	if err != nil {
		return m.replyError(e, fmt.Errorf("shamir.combine: %w", err))
	}

	payload, _ := json.Marshal(combineResult{SecretHex: hex.EncodeToString(secret)})
	m.dispatcher.Dispatch(kernel.ReplyEvent("shamir", kernel.EvShamirResult, e, payload)) //nolint:errcheck
	log.Printf("[shamir:COMBINE] shares=%d secretLen=%d", len(shares), len(secret))
	return nil
}

// ─── Error helper ─────────────────────────────────────────────────────────────

func (m *Module) replyError(e kernel.Event, err error) error {
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	m.dispatcher.Dispatch(kernel.ReplyEvent("shamir", kernel.EvShamirResult, e, payload)) //nolint:errcheck
	return err
}
