package sdk

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gorillaws "github.com/gorilla/websocket"
	"github.com/grimlocker/grimdb/gql"
)

var upgrader = gorillaws.Upgrader{}

func newTestServer(t *testing.T, handler func(*gorillaws.Conn)) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		conn.WriteMessage(gorillaws.BinaryMessage, []byte{0, 0, 0, 0, byte(gql.OpcodeResult), '{', '}', 0, 0, 0, 0, byte(gql.OpcodeResult)})
		handler(conn)
	}))
	wsURL := "ws" + server.URL[4:] + "/ws"
	return server, wsURL
}

func readFrame(t *testing.T, conn *gorillaws.Conn) (*gql.Frame, []byte) {
	t.Helper()
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	frame, err := gql.DecodeFrame(data)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	return frame, data
}

func writeResultFrame(t *testing.T, conn *gorillaws.Conn, entries []gql.GQLEntry, total uint32) {
	t.Helper()
	result := &gql.GQLResult{
		Success:    true,
		Entries:    entries,
		TotalCount: total,
	}
	frame := gql.NewResultFrame(result)
	conn.WriteMessage(gorillaws.BinaryMessage, frame.Encode())
}

func writeErrorFrame(t *testing.T, conn *gorillaws.Conn, code int32, msg string) {
	t.Helper()
	frame := gql.NewErrorFrame(code, msg)
	conn.WriteMessage(gorillaws.BinaryMessage, frame.Encode())
}

func sampleEntry(id, title, category string) gql.GQLEntry {
	return gql.GQLEntry{
		ID:        id,
		Title:     title,
		Category:  category,
		CreatedAt: 1234567890,
		UpdatedAt: 1234567891,
		Fields: map[string]string{
			"username": "testuser",
			"password": "testpass",
		},
	}
}

func TestConnect(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {})
	defer server.Close()

	client, err := DialGQL(context.Background(), wsURL)
	if err != nil {
		t.Fatalf("DialGQL: %v", err)
	}
	defer client.Close()
}

func TestListEntries(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		entries := []gql.GQLEntry{
			sampleEntry("e1", "Entry One", "PASSWORD"),
			sampleEntry("e2", "Entry Two", "SSH_KEY"),
		}
		writeResultFrame(t, conn, entries, uint32(len(entries)))
	})
	defer server.Close()

	client, err := DialGQL(context.Background(), wsURL)
	if err != nil {
		t.Fatalf("DialGQL: %v", err)
	}
	defer client.Close()

	entries, err := client.ListEntries(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].ID != "e1" {
		t.Errorf("first entry ID: expected e1, got %s", entries[0].ID)
	}
	if entries[1].Category != "SSH_KEY" {
		t.Errorf("second entry category: expected SSH_KEY, got %s", entries[1].Category)
	}
}

func TestGetEntry(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, []gql.GQLEntry{sampleEntry("e99", "Target", "PASSWORD")}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	entry, err := client.GetEntry(context.Background(), "default", "e99")
	if err != nil {
		t.Fatalf("GetEntry: %v", err)
	}
	if entry.ID != "e99" || entry.Title != "Target" {
		t.Errorf("unexpected entry: %+v", entry)
	}
}

func TestGetEntryNotFound(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, nil, 0)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	_, err := client.GetEntry(context.Background(), "default", "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestCreateEntry(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, []gql.GQLEntry{sampleEntry("new1", "New Entry", "PASSWORD")}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	fields := map[string]string{"username": "alice", "password": "s3cret"}
	entry, err := client.CreateEntry(context.Background(), "default", "New Entry", "PASSWORD", fields)
	if err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	if entry.ID != "new1" {
		t.Errorf("expected new1, got %s", entry.ID)
	}
}

func TestUpdateEntry(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, nil, 0)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	err := client.UpdateEntry(context.Background(), "default", "e1", "Updated Title", map[string]string{"notes": "updated"})
	if err != nil {
		t.Fatalf("UpdateEntry: %v", err)
	}
}

func TestDeleteEntry(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, nil, 0)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	err := client.DeleteEntry(context.Background(), "default", "e1")
	if err != nil {
		t.Fatalf("DeleteEntry: %v", err)
	}
}

func TestCreatePassword(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		entry := sampleEntry("p1", "GitHub", "PASSWORD")
		writeResultFrame(t, conn, []gql.GQLEntry{entry}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	pe := &PasswordEntry{
		Title:    "GitHub",
		Username: "alice",
		Password: "s3cret",
		URL:      "https://github.com",
		Notes:    "main account",
	}
	id, err := client.CreatePassword(context.Background(), "default", pe)
	if err != nil {
		t.Fatalf("CreatePassword: %v", err)
	}
	if id != "p1" {
		t.Errorf("expected p1, got %s", id)
	}
}

func TestListPasswords(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		entries := []gql.GQLEntry{
			{ID: "p1", Title: "GitHub", Category: "PASSWORD", Fields: map[string]string{"username": "alice", "password": "sec1"}},
			{ID: "p2", Title: "GitLab", Category: "PASSWORD", Fields: map[string]string{"username": "bob", "password": "sec2"}},
		}
		writeResultFrame(t, conn, entries, 2)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	passwords, err := client.ListPasswords(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListPasswords: %v", err)
	}
	if len(passwords) != 2 {
		t.Fatalf("expected 2 passwords, got %d", len(passwords))
	}
}

func TestCreateSSHKey(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, []gql.GQLEntry{sampleEntry("sk1", "My SSH Key", "SSH_KEY")}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	k := &SSHKeyEntry{
		Title:     "My SSH Key",
		PublicKey: "ssh-ed25519 AAAAC3...",
		Comment:   "laptop",
		Algorithm: "ed25519",
	}
	id, err := client.CreateSSHKey(context.Background(), "default", k)
	if err != nil {
		t.Fatalf("CreateSSHKey: %v", err)
	}
	if id != "sk1" {
		t.Errorf("expected sk1, got %s", id)
	}
}

func TestListSSHKeys(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, []gql.GQLEntry{sampleEntry("sk1", "Key", "SSH_KEY")}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	keys, err := client.ListSSHKeys(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListSSHKeys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}
}

func TestCreateCertificate(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, []gql.GQLEntry{sampleEntry("c1", "My Cert", "CERTIFICATE")}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	c := &CertificateEntry{
		Title:       "My Cert",
		Domain:      "example.com",
		Certificate: "-----BEGIN CERTIFICATE-----...",
		PrivateKey:  "-----BEGIN PRIVATE KEY-----...",
	}
	id, err := client.CreateCertificate(context.Background(), "default", c)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	if id != "c1" {
		t.Errorf("expected c1, got %s", id)
	}
}

func TestListCertificates(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, []gql.GQLEntry{sampleEntry("c1", "Cert", "CERTIFICATE")}, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	certs, err := client.ListCertificates(context.Background(), "default")
	if err != nil {
		t.Fatalf("ListCertificates: %v", err)
	}
	if len(certs) != 1 {
		t.Errorf("expected 1 cert, got %d", len(certs))
	}
}

func TestSearchEntries(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		entries := []gql.GQLEntry{sampleEntry("e1", "GitHub", "PASSWORD")}
		writeResultFrame(t, conn, entries, 1)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	payload := map[string]string{"query": "git"}
	raw, err := client.sendCommand(context.Background(), "default", gql.OpSearchEntries, payload)
	if err != nil {
		t.Fatalf("SearchEntries: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil raw response")
	}
}

func TestHealthCheck(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		writeResultFrame(t, conn, nil, 0)
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
}

func TestListWorkspaces(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `[{"id":"ws1","name":"Personal","is_default":true,"created_at":1}]`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	workspaces, err := client.ListWorkspaces(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace, got %d", len(workspaces))
	}
}

func TestListSyncPeers(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"peers":[{"id":"p1","name":"peer1","address":"192.168.1.5","connected":true,"last_seen":1}],"last_sync_at":0}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	status, err := client.ListSyncPeers(context.Background())
	if err != nil {
		t.Fatalf("ListSyncPeers: %v", err)
	}
	if len(status.Peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(status.Peers))
	}
}

func TestTriggerSync(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(`{}`))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	err := client.TriggerSync(context.Background())
	if err != nil {
		t.Fatalf("TriggerSync: %v", err)
	}
}

func TestListAuditEvents(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `[{"timestamp":1,"level":"INFO","module":"auth","message":"vault unlocked","subject_id":""}]`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	events, err := client.ListAuditEvents(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 audit event, got %d", len(events))
	}
}

func TestGenerateSSHKey(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"public_key":"ssh-ed25519 AAAAC3...","fingerprint":"SHA256:abc","entry_id":"e99"}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	result, err := client.GenerateSSHKey(context.Background(), "", true)
	if err != nil {
		t.Fatalf("GenerateSSHKey: %v", err)
	}
	if result.PublicKey == "" {
		t.Error("expected non-empty public key")
	}
}

func TestGetRecoveryPhrase(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"phrase":"abandon ability able about above absent absorb abstract..."}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	phrase, err := client.GetRecoveryPhrase(context.Background(), "master-password")
	if err != nil {
		t.Fatalf("GetRecoveryPhrase: %v", err)
	}
	if phrase == "" {
		t.Error("expected non-empty recovery phrase")
	}
}

func TestErrorHandling(t *testing.T) {
	t.Run("error frame", func(t *testing.T) {
		server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
			_, _ = readFrame(t, conn)
			writeErrorFrame(t, conn, -101, "vault is locked")
		})
		defer server.Close()

		client, _ := DialGQL(context.Background(), wsURL)
		defer client.Close()

		_, err := client.ListEntries(context.Background(), "default")
		if err == nil {
			t.Fatal("expected error for locked vault")
		}
		if !strings.Contains(err.Error(), "vault is locked") {
			t.Errorf("expected 'vault is locked', got: %v", err)
		}
	})

	t.Run("invalid frame", func(t *testing.T) {
		server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
			_, _ = readFrame(t, conn)
			conn.WriteMessage(gorillaws.BinaryMessage, []byte{0x00, 0x00, 0x00, 0x00, 0xFF})
		})
		defer server.Close()

		client, _ := DialGQL(context.Background(), wsURL)
		defer client.Close()

		_, err := client.ListEntries(context.Background(), "default")
		if err == nil {
			t.Fatal("expected error for invalid frame")
		}
	})

	t.Run("unparseable error payload", func(t *testing.T) {
		server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
			_, _ = readFrame(t, conn)
			frame := &gql.Frame{
				Version:     gql.Version,
				Opcode:      gql.OpcodeError,
				PayloadSize: 10,
				Payload:     []byte("not-valid-json!"),
			}
			conn.WriteMessage(gorillaws.BinaryMessage, frame.Encode())
		})
		defer server.Close()

		client, _ := DialGQL(context.Background(), wsURL)
		defer client.Close()

		_, err := client.ListEntries(context.Background(), "default")
		if err == nil {
			t.Fatal("expected error for unparseable payload")
		}
	})
}

func TestClosedClient(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	client.Close()

	_, err := client.ListEntries(context.Background(), "default")
	if err == nil {
		t.Fatal("expected error for closed client")
	}
}

func TestListFolder(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"folder_id":"root","folder_name":"root","files":[],"folders":[]}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	listing, err := client.ListFolder(context.Background(), "root")
	if err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	if listing == nil {
		t.Fatal("expected non-nil listing")
	}
}

func TestCreateFolder(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"id":"f1","name":"Notes","parent_id":"","created_at":1,"updated_at":1}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	folder, err := client.CreateFolder(context.Background(), "Notes", "")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if folder.Name != "Notes" {
		t.Errorf("expected 'Notes', got %s", folder.Name)
	}
}

func TestMoveFile(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(`{}`))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	err := client.MoveFile(context.Background(), "mb1", "folder1")
	if err != nil {
		t.Fatalf("MoveFile: %v", err)
	}
}

func TestUploadFile(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"id":"f1","manifest_block_id":"mb1","file_name":"doc.txt","mime_type":"text/plain","total_size":12,"folder_id":"","created_at":1}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	entry, err := client.UploadFile(context.Background(), "default", []byte("hello world"), "doc.txt", "text/plain", "")
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	if entry.FileName != "doc.txt" {
		t.Errorf("expected doc.txt, got %s", entry.FileName)
	}
}

func TestDownloadFile(t *testing.T) {
	server, wsURL := newTestServer(t, func(conn *gorillaws.Conn) {
		_, _ = readFrame(t, conn)
		payload := `{"data":"` + b64Enc("hello world") + `"}`
		conn.WriteMessage(gorillaws.BinaryMessage, newRawOKFrame(payload))
	})
	defer server.Close()

	client, _ := DialGQL(context.Background(), wsURL)
	defer client.Close()

	data, err := client.DownloadFile(context.Background(), "mb1")
	if err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if data == nil || len(data) == 0 {
		t.Fatal("expected non-empty download data")
	}
}

func newRawOKFrame(payloadJSON string) []byte {
	result := &gql.GQLResult{
		Success: true,
		Entries: []gql.GQLEntry{{
			Fields: map[string]string{"payload": payloadJSON},
		}},
	}
	frame := gql.NewResultFrame(result)
	return frame.Encode()
}

func b64Enc(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}
