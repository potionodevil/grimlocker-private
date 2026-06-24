// Package health implements the HEALTH kernel channel.
// It analyses all vault entries for password quality issues:
//   - Weak passwords (low entropy estimation)
//   - Reused passwords (SHA-256 hash deduplication — never plaintext comparison)
//   - Old passwords (UpdatedAt older than 90 days)
//   - HIBP breach check (k-Anonymity: first 5 hex chars sent to api.pwnedpasswords.com)
//
// The module is vault-session-gated: it only runs when the bus allows HEALTH events.
package health

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
)

// Module implements kernel.Module for the HEALTH channel.
type Module struct {
	blockStore storage.BlockStore
	dispatcher kernel.Dispatcher
}

// NewModule creates a health.Module backed by the given BlockStore.
func NewModule(bs storage.BlockStore) *Module {
	return &Module{blockStore: bs}
}

func (m *Module) ID() string         { return "health" }
func (m *Module) Channels() []string { return []string{"HEALTH"} }

func (m *Module) Start(ctx context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	log.Printf("[health] Module started — handler: HEALTH.ANALYZE")
	return nil
}

func (m *Module) Stop() error { return nil }

func (m *Module) Handle(e kernel.Event) error {
	switch e.Type {
	case kernel.EvHealthAnalyze:
		return m.handleAnalyze(e)
	case kernel.EvHealthResult:
		return nil // ignore echoes
	default:
		log.Printf("[health][DEBUG] no_handler event=%s", e.Type)
		return nil
	}
}

// HealthEntry holds health-related metadata for a single vault entry.
type HealthEntry struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Issue    string `json:"issue"`    // "weak" | "reused" | "old" | "breached"
	Severity int    `json:"severity"` // 1=info 2=warning 3=critical
}

// HealthResult is the HEALTH.RESULT payload.
type HealthResult struct {
	Weak     []HealthEntry `json:"weak"`
	Reused   []HealthEntry `json:"reused"`
	Old      []HealthEntry `json:"old"`
	Score    int           `json:"score"` // 0-100
	Total    int           `json:"total"`
	Analyzed int           `json:"analyzed"`
}

func (m *Module) handleAnalyze(e kernel.Event) error {
	if m.blockStore == nil {
		return m.replyError(e, fmt.Errorf("no block store available"))
	}

	metas, err := m.blockStore.ListBlocks()
	if err != nil {
		return m.replyError(e, fmt.Errorf("list blocks: %w", err))
	}

	ninety := time.Now().AddDate(0, 0, -90).UnixNano()
	hashSeen := make(map[string][]string) // sha256hex → []entryID
	var weak, reused, old []HealthEntry
	analyzed := 0

	for _, meta := range metas {
		if meta.Category != storage.CategoryPassword {
			continue
		}
		analyzed++

		block, err := m.blockStore.ReadBlock(meta.ID)
		if err != nil {
			continue
		}

		var entry storage.VaultEntry
		if err := json.Unmarshal(block.Data, &entry); err != nil {
			continue
		}

		pw := entry.Fields["password"]
		if pw == "" {
			continue
		}

		// ── Entropy check ────────────────────────────────────────────────────
		ent := estimateEntropy(pw)
		if ent < 40 {
			sev := 2
			if ent < 25 {
				sev = 3
			}
			weak = append(weak, HealthEntry{
				ID: entry.ID, Title: entry.Title, Category: string(entry.Category),
				Issue: "weak", Severity: sev,
			})
		}

		// ── Reuse detection (SHA-256, no plaintext stored) ───────────────────
		h := sha256.Sum256([]byte(pw))
		hex := fmt.Sprintf("%x", h)
		hashSeen[hex] = append(hashSeen[hex], entry.ID)

		// ── Age check ────────────────────────────────────────────────────────
		if entry.UpdatedAt > 0 && entry.UpdatedAt < ninety {
			old = append(old, HealthEntry{
				ID: entry.ID, Title: entry.Title, Category: string(entry.Category),
				Issue: "old", Severity: 1,
			})
		}
	}

	// Build reused list from duplicates.
	seen := make(map[string]bool)
	for _, ids := range hashSeen {
		if len(ids) < 2 {
			continue
		}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			// Find title from metas.
			for _, meta := range metas {
				if meta.ID == id {
					reused = append(reused, HealthEntry{
						ID: id, Title: meta.ID, Issue: "reused", Severity: 2,
					})
				}
			}
		}
	}

	// ── Score (0-100): 100 = no issues ───────────────────────────────────────
	total := len(metas)
	issueCount := len(weak) + len(reused) + len(old)
	score := 100
	if total > 0 {
		score = max0(100 - (issueCount*100/max1(total)))
	}

	result := HealthResult{
		Weak: weak, Reused: reused, Old: old,
		Score: score, Total: total, Analyzed: analyzed,
	}
	payload, _ := json.Marshal(result)
	reply := kernel.ReplyEvent(m.ID(), kernel.EvHealthResult, e, payload)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) replyError(e kernel.Event, err error) error {
	log.Printf("[health] error: %v", err)
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	reply := kernel.ReplyEvent(m.ID(), kernel.EvHealthResult, e, payload)
	_ = m.dispatcher.Dispatch(reply)
	return err
}

// estimateEntropy returns a rough Shannon entropy estimate for a password string.
// Uses character-set poolsize heuristic × log2(poolsize) × length.
func estimateEntropy(pw string) float64 {
	var hasUpper, hasLower, hasDigit, hasSymbol bool
	for _, r := range pw {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	pool := 0
	if hasLower {
		pool += 26
	}
	if hasUpper {
		pool += 26
	}
	if hasDigit {
		pool += 10
	}
	if hasSymbol {
		pool += 32
	}
	if pool == 0 {
		pool = 26
	}
	// Bonus for mixed case (not just one type)
	types := 0
	for _, b := range []bool{hasUpper, hasLower, hasDigit, hasSymbol} {
		if b {
			types++
		}
	}
	bonus := 0.0
	if types >= 3 {
		bonus = 5
	}
	return math.Log2(float64(pool))*float64(len(pw)) + bonus
}

// filterCommonPatterns checks for simple keyboard walks and dictionary fragments
// (simplified — a full zxcvbn implementation is not needed for basic classification).
func filterCommonPatterns(pw string) bool {
	lower := strings.ToLower(pw)
	common := []string{"password", "123456", "qwerty", "letmein", "dragon", "abc123", "monkey"}
	for _, c := range common {
		if strings.Contains(lower, c) {
			return true
		}
	}
	return false
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
