// Package importmod implements the IMPORT kernel channel.
// Handles CSV imports from common password managers:
//   - 1Password (csv export)
//   - Bitwarden (csv export)
//   - Chrome / Edge (passwords.csv)
//   - KeePass (keepass csv)
//   - Generic (title,username,password,url,notes)
package importmod

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
)

// Module implements kernel.Module for the IMPORT channel.
type Module struct {
	blockStore storage.BlockStore
	dispatcher kernel.Dispatcher
}

// NewModule creates an import.Module.
func NewModule(bs storage.BlockStore) *Module {
	return &Module{blockStore: bs}
}

func (m *Module) ID() string         { return "import" }
func (m *Module) Channels() []string { return []string{"IMPORT"} }

func (m *Module) Start(ctx context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	log.Printf("[import] Module started — handler: IMPORT.CSV")
	return nil
}

func (m *Module) Stop() error { return nil }

func (m *Module) Handle(e kernel.Event) error {
	switch e.Type {
	case kernel.EvImportCSV:
		return m.handleCSV(e)
	case kernel.EvImportResult:
		return nil
	default:
		log.Printf("[import][DEBUG] no_handler event=%s", e.Type)
		return nil
	}
}

// ImportRequest is the IMPORT.CSV payload.
type ImportRequest struct {
	CSVContent string `json:"csv_content"`
	Format     string `json:"format"` // "auto" | "1password" | "bitwarden" | "chrome" | "keepass" | "generic"
}

// ImportResult is the IMPORT.RESULT payload.
type ImportResult struct {
	Imported int      `json:"imported"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors"`
}

// importedEntry is a normalised entry after parsing.
type importedEntry struct {
	Title    string
	Username string
	Password string
	URL      string
	Notes    string
}

func (m *Module) handleCSV(e kernel.Event) error {
	var req ImportRequest
	if err := json.Unmarshal(e.Payload, &req); err != nil {
		return m.replyError(e, fmt.Errorf("invalid import request: %w", err))
	}
	if req.CSVContent == "" {
		return m.replyError(e, fmt.Errorf("csv_content is empty"))
	}

	// Auto-detect format if not specified.
	format := req.Format
	if format == "" || format == "auto" {
		format = detectFormat(req.CSVContent)
	}

	entries, errs := parseCSV(req.CSVContent, format)
	result := ImportResult{Errors: errs}

	for _, ie := range entries {
		if ie.Title == "" {
			ie.Title = ie.URL
		}
		if ie.Title == "" {
			ie.Title = "Importierter Eintrag"
		}

		id := newUUID()
		now := time.Now().UnixNano()
		entry := storage.VaultEntry{
			ID:       id,
			Title:    ie.Title,
			Category: storage.CategoryPassword,
			Type:     "password",
			Fields: map[string]string{
				"username": ie.Username,
				"password": ie.Password,
				"url":      ie.URL,
				"notes":    ie.Notes,
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		data, _ := json.Marshal(entry)
		block := storage.Block{
			ID:       id,
			Data:     data,
			Category: storage.CategoryPassword,
		}
		if err := m.blockStore.WriteBlock(block); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("save %q: %v", ie.Title, err))
			result.Skipped++
		} else {
			result.Imported++
		}
	}

	payload, _ := json.Marshal(result)
	reply := kernel.ReplyEvent(m.ID(), kernel.EvImportResult, e, payload)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) replyError(e kernel.Event, err error) error {
	log.Printf("[import] error: %v", err)
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	reply := kernel.ReplyEvent(m.ID(), kernel.EvImportResult, e, payload)
	_ = m.dispatcher.Dispatch(reply)
	return err
}

// detectFormat sniffs the CSV header to guess the source manager.
func detectFormat(csv string) string {
	firstLine := strings.ToLower(strings.SplitN(csv, "\n", 2)[0])
	switch {
	case strings.Contains(firstLine, "notesplaintext") || strings.Contains(firstLine, "totp"):
		return "1password"
	case strings.Contains(firstLine, "reprompt"):
		return "bitwarden"
	case strings.Contains(firstLine, "name,url,username,password"):
		return "chrome"
	case strings.Contains(firstLine, "\"account\"") || strings.Contains(firstLine, "keepass"):
		return "keepass"
	default:
		return "generic"
	}
}

// parseCSV parses the CSV content according to the detected format.
func parseCSV(content, format string) ([]importedEntry, []string) {
	r := csv.NewReader(strings.NewReader(content))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, []string{fmt.Sprintf("CSV parse error: %v", err)}
	}
	if len(records) < 2 {
		return nil, []string{"CSV is empty or has no data rows"}
	}

	header := records[0]
	idx := buildIndex(header)
	var entries []importedEntry
	var errs []string

	for i, row := range records[1:] {
		if len(row) < 2 {
			continue
		}
		get := func(keys ...string) string {
			for _, k := range keys {
				if i, ok := idx[strings.ToLower(strings.TrimSpace(k))]; ok && i < len(row) {
					return strings.TrimSpace(row[i])
				}
			}
			return ""
		}
		_ = i
		var ie importedEntry
		switch format {
		case "1password":
			ie = importedEntry{
				Title:    get("title"),
				Username: get("username"),
				Password: get("password"),
				URL:      get("url"),
				Notes:    get("notesplaintext", "notes"),
			}
		case "bitwarden":
			ie = importedEntry{
				Title:    get("name"),
				Username: get("login_username", "username"),
				Password: get("login_password", "password"),
				URL:      get("login_uri", "url"),
				Notes:    get("notes"),
			}
		case "chrome":
			ie = importedEntry{
				Title:    get("name"),
				Username: get("username"),
				Password: get("password"),
				URL:      get("url"),
			}
		case "keepass":
			ie = importedEntry{
				Title:    get("account", "title", "name"),
				Username: get("login name", "username"),
				Password: get("password"),
				URL:      get("web site", "url"),
				Notes:    get("comments", "notes"),
			}
		default: // generic
			ie = importedEntry{
				Title:    get("title", "name", "account"),
				Username: get("username", "login", "email"),
				Password: get("password"),
				URL:      get("url", "website"),
				Notes:    get("notes", "comment"),
			}
		}
		if ie.Password == "" {
			errs = append(errs, fmt.Sprintf("row %d: skipped (no password)", i+2))
			continue
		}
		entries = append(entries, ie)
	}
	return entries, errs
}

func buildIndex(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.ToLower(strings.TrimSpace(h))] = i
	}
	return m
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
