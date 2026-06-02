// grim — Grimlocker CLI
//
// A standalone Go binary that wraps the full Grimlocker API as a CLI.
// Uses POST to /api/v1 with JSON body {"action":"...", "payload":{...}}
// and header X-Grimlocker-Token.
//
// Build: go build -o grim .
// Base URL from GRIMLOCKER_URL env var (default http://127.0.0.1:36353)
// Token from GRIMLOCKER_TOKEN env var
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const version = "1.0.0"

var (
	baseURL   string
	token     string
	pretty    bool
	silent    bool
	httpClent = &http.Client{Timeout: 60 * time.Second}
)

func main() {
	baseURL = os.Getenv("GRIMLOCKER_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:36353"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")
	token = os.Getenv("GRIMLOCKER_TOKEN")

	// Extract --pretty and --silent from anywhere in args
	args := os.Args[1:]
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--pretty":
			pretty = true
		case "--silent":
			silent = true
		default:
			filtered = append(filtered, a)
		}
	}

	if len(filtered) < 1 {
		printUsage()
		os.Exit(1)
	}

	cmd := filtered[0]
	cmdArgs := filtered[1:]

	// Commands that don't need the API
	switch cmd {
	case "version", "--version", "-v":
		if silent {
			fmt.Println(version)
		} else {
			fmt.Printf("grim %s\n", version)
		}
		return
	case "help", "--help", "-h":
		printUsage()
		return
	case "ssh-keygen":
		cmdSSHKeyGen(cmdArgs)
		return
	}

	// All other commands need the API
	exitCode := dispatch(cmd, cmdArgs)
	os.Exit(exitCode)
}

func dispatch(cmd string, args []string) int {
	switch cmd {
	case "unlock":
		return cmdUnlock(args)
	case "lock":
		return cmdLock()
	case "status":
		return cmdStatus()
	case "entries":
		if len(args) < 1 {
			printEntriesUsage()
			return 1
		}
		return cmdEntries(args[0], args[1:])
	case "files":
		if len(args) < 1 {
			printFilesUsage()
			return 1
		}
		return cmdFiles(args[0], args[1:])
	case "workspaces":
		if len(args) < 1 {
			printWorkspacesUsage()
			return 1
		}
		return cmdWorkspaces(args[0], args[1:])
	case "sync":
		if len(args) < 1 {
			printSyncUsage()
			return 1
		}
		return cmdSync(args[0])
	case "audit":
		return cmdAudit(args)
	case "health":
		return cmdHealth()
	default:
		fmt.Fprintf(os.Stderr, "grim: unknown command %q — run 'grim help'\n", cmd)
		return 1
	}
}

// ─── API helpers ─────────────────────────────────────────────────────────────

type apiRequest struct {
	Action  string      `json:"action"`
	Payload interface{} `json:"payload,omitempty"`
}

type apiResponse struct {
	OK        bool            `json:"ok"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     string          `json:"error,omitempty"`
	ErrorCode int             `json:"error_code,omitempty"`
}

func apiCall(action string, payload interface{}) (*apiResponse, error) {
	body, err := json.Marshal(apiRequest{Action: action, Payload: payload})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Grimlocker-Token", token)
	}

	resp, err := httpClent.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBody))
	}

	var ar apiResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if !ar.OK {
		msg := ar.Error
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return &ar, nil
}

func output(v interface{}) {
	if silent {
		return
	}
	if pretty {
		out, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Println(v)
			return
		}
		fmt.Println(string(out))
		return
	}
	// Default: compact JSON
	out, err := json.Marshal(v)
	if err != nil {
		fmt.Println(v)
		return
	}
	fmt.Println(string(out))
}

func outputResp(resp *apiResponse) {
	if silent {
		return
	}
	if pretty {
		outputRaw(resp)
		return
	}
	// Compact JSON
	out, _ := json.Marshal(resp)
	fmt.Println(string(out))
}

func outputRaw(v interface{}) {
	if silent {
		return
	}
	out, _ := json.Marshal(v)
	fmt.Println(string(out))
}

func outputPretty(v interface{}) {
	if silent {
		return
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}

func fail(msg string) int {
	fmt.Fprintf(os.Stderr, "grim: %s\n", msg)
	return 1
}

// ─── Vault Commands ──────────────────────────────────────────────────────────

func cmdUnlock(args []string) int {
	password := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--password", "-p":
			if i+1 < len(args) {
				password = args[i+1]
				i++
			}
		default:
			return fail("usage: grim unlock --password <password>")
		}
	}
	if password == "" {
		return fail("usage: grim unlock --password <password>")
	}

	resp, err := apiCall("vault.unlock", map[string]string{"password": password})
	if err != nil {
		return fail(err.Error())
	}
	if !silent {
		fmt.Println("Vault unlocked.")
	}
	outputResp(resp)
	return 0
}

func cmdLock() int {
	resp, err := apiCall("vault.logout", map[string]string{})
	if err != nil {
		return fail(err.Error())
	}
	if !silent {
		fmt.Println("Vault locked.")
	}
	outputResp(resp)
	return 0
}

func cmdStatus() int {
	resp, err := apiCall("vault.status", map[string]string{})
	if err != nil {
		return fail(err.Error())
	}
	outputResp(resp)
	return 0
}

// ─── Entry Commands ──────────────────────────────────────────────────────────

func cmdEntries(subcmd string, args []string) int {
	switch subcmd {
	case "list":
		return cmdEntryList(args)
	case "get":
		return cmdEntryGet(args)
	case "create":
		if len(args) < 1 {
			printEntriesCreateUsage()
			return 1
		}
		return cmdEntryCreate(args[0], args[1:])
	case "update":
		return cmdEntryUpdate(args)
	case "delete":
		return cmdEntryDelete(args)
	case "search":
		return cmdEntrySearch(args)
	default:
		printEntriesUsage()
		return 1
	}
}

func cmdEntryList(args []string) int {
	category := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--category", "-c":
			if i+1 < len(args) {
				category = args[i+1]
				i++
			}
		}
	}

	resp, err := apiCall("entry.query", map[string]string{"category": category})
	if err != nil {
		return fail(err.Error())
	}

	if silent {
		return 0
	}

	if pretty {
		outputPrettyTable(resp, category)
		return 0
	}
	outputResp(resp)
	return 0
}

func cmdEntryGet(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim entries get <id>")
	}
	id := args[0]

	resp, err := apiCall("entry.read", map[string]string{"id": id})
	if err != nil {
		return fail(err.Error())
	}
	if pretty {
		outputPrettyEntry(resp)
	} else {
		outputResp(resp)
	}
	return 0
}

func cmdEntryCreate(entryType string, args []string) int {
	switch entryType {
	case "password":
		return cmdEntryCreatePassword(args)
	case "ssh-key":
		return cmdEntryCreateSSHKey(args)
	case "certificate":
		return cmdEntryCreateCert(args)
	default:
		return fail("usage: grim entries create [password|ssh-key|certificate] ...")
	}
}

func cmdEntryCreatePassword(args []string) int {
	var title, username, password, url, notes string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title", "-t":
			if i+1 < len(args) { title = args[i+1]; i++ }
		case "--username", "-u":
			if i+1 < len(args) { username = args[i+1]; i++ }
		case "--password", "-p":
			if i+1 < len(args) { password = args[i+1]; i++ }
		case "--url":
			if i+1 < len(args) { url = args[i+1]; i++ }
		case "--notes", "-n":
			if i+1 < len(args) { notes = args[i+1]; i++ }
		}
	}
	if title == "" || username == "" || password == "" {
		return fail("usage: grim entries create password --title <t> --username <u> --password <p> [--url <u>] [--notes <n>]")
	}

	fields := map[string]string{
		"username": username,
		"password": password,
	}
	if url != "" {
		fields["url"] = url
	}
	if notes != "" {
		fields["notes"] = notes
	}

	resp, err := apiCall("entry.create", map[string]interface{}{
		"title":    title,
		"type":     "password",
		"category": "PASSWORD",
		"fields":   fields,
	})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		outputPrettyEntry(resp)
	} else {
		if !silent {
			fmt.Printf("Password entry %q created.\n", title)
		}
		outputResp(resp)
	}
	return 0
}

func cmdEntryCreateSSHKey(args []string) int {
	var title, publicKey, username, passphrase string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title", "-t":
			if i+1 < len(args) { title = args[i+1]; i++ }
		case "--public-key", "-k":
			if i+1 < len(args) { publicKey = args[i+1]; i++ }
		case "--username", "-u":
			if i+1 < len(args) { username = args[i+1]; i++ }
		case "--passphrase":
			if i+1 < len(args) { passphrase = args[i+1]; i++ }
		}
	}
	if title == "" || publicKey == "" {
		return fail("usage: grim entries create ssh-key --title <t> --public-key <k> [--username <u>] [--passphrase <p>]")
	}

	fields := map[string]string{
		"publicKey": publicKey,
	}
	if username != "" {
		fields["username"] = username
	}
	if passphrase != "" {
		fields["passphraseProtected"] = "true"
	}

	resp, err := apiCall("entry.create", map[string]interface{}{
		"title":    title,
		"type":     "ssh",
		"category": "SSH_KEY",
		"fields":   fields,
	})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		outputPrettyEntry(resp)
	} else {
		if !silent {
			fmt.Printf("SSH key entry %q created.\n", title)
		}
		outputResp(resp)
	}
	return 0
}

func cmdEntryCreateCert(args []string) int {
	var title, domain, cert, privateKey string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title", "-t":
			if i+1 < len(args) { title = args[i+1]; i++ }
		case "--domain", "-d":
			if i+1 < len(args) { domain = args[i+1]; i++ }
		case "--cert", "-c":
			if i+1 < len(args) { cert = args[i+1]; i++ }
		case "--private-key":
			if i+1 < len(args) { privateKey = args[i+1]; i++ }
		}
	}
	if title == "" || domain == "" || cert == "" || privateKey == "" {
		return fail("usage: grim entries create certificate --title <t> --domain <d> --cert <c> --private-key <k>")
	}

	resp, err := apiCall("entry.create", map[string]interface{}{
		"title":    title,
		"type":     "certificate",
		"category": "CERTIFICATE",
		"fields": map[string]string{
			"domain":     domain,
			"cert":       cert,
			"privateKey": privateKey,
		},
	})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		outputPrettyEntry(resp)
	} else {
		if !silent {
			fmt.Printf("Certificate entry %q created.\n", title)
		}
		outputResp(resp)
	}
	return 0
}

func cmdEntryUpdate(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim entries update <id> --field key=value [--field key2=value2...]")
	}
	id := args[0]

	fields := map[string]string{}
	title := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--field" || args[i] == "-f" {
			if i+1 < len(args) {
				kv := args[i+1]
				i++
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) == 2 {
					fields[parts[0]] = parts[1]
				}
			}
		} else if args[i] == "--title" || args[i] == "-t" {
			if i+1 < len(args) {
				title = args[i+1]
				i++
			}
		}
	}

	if len(fields) == 0 && title == "" {
		return fail("usage: grim entries update <id> --field key=value [--field key2=value2...]")
	}

	resp, err := apiCall("entry.update", map[string]interface{}{
		"id":     id,
		"title":  title,
		"fields": fields,
	})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		outputPrettyEntry(resp)
	} else {
		if !silent {
			fmt.Printf("Entry %s updated.\n", id)
		}
		outputResp(resp)
	}
	return 0
}

func cmdEntryDelete(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim entries delete <id>")
	}

	resp, err := apiCall("entry.delete", map[string]string{"id": args[0]})
	if err != nil {
		return fail(err.Error())
	}
	if !silent {
		fmt.Printf("Entry %s deleted.\n", args[0])
	}
	outputResp(resp)
	return 0
}

func cmdEntrySearch(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim entries search <query> [--category <c>]")
	}
	query := strings.ToLower(args[0])
	category := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--category" || args[i] == "-c" {
			if i+1 < len(args) {
				category = args[i+1]
				i++
			}
		}
	}

	// Fetch all entries, filter client-side
	resp, err := apiCall("entry.query", map[string]string{"category": category})
	if err != nil {
		return fail(err.Error())
	}

	if silent {
		return 0
	}

	// Filter results by query in title
	var payload struct {
		Entries  []map[string]interface{} `json:"entries"`
		Category string                   `json:"category"`
		Count    int                      `json:"count"`
	}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		// Try direct array
		var entries []map[string]interface{}
		if err := json.Unmarshal(resp.Payload, &entries); err != nil {
			outputResp(resp)
			return 0
		}
		payload.Entries = entries
	}

	var matches []map[string]interface{}
	for _, e := range payload.Entries {
		if title, ok := e["title"].(string); ok && strings.Contains(strings.ToLower(title), query) {
			matches = append(matches, e)
		} else if id, ok := e["id"].(string); ok && strings.Contains(strings.ToLower(id), query) {
			matches = append(matches, e)
		}
	}

	if pretty {
		fmt.Printf("Search results for %q (%d matches):\n", args[0], len(matches))
		for _, m := range matches {
			printEntrySummary(m)
		}
	} else {
		output(matches)
	}
	return 0
}

// ─── File Commands ───────────────────────────────────────────────────────────

func cmdFiles(subcmd string, args []string) int {
	switch subcmd {
	case "upload":
		return cmdFileUpload(args)
	case "download":
		return cmdFileDownload(args)
	case "list-folder":
		return cmdFileListFolder(args)
	case "create-folder":
		return cmdFileCreateFolder(args)
	case "rename-folder":
		return cmdFileRenameFolder(args)
	case "delete-folder":
		return cmdFileDeleteFolder(args)
	case "move":
		return cmdFileMove(args)
	default:
		printFilesUsage()
		return 1
	}
}

func cmdFileUpload(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim files upload <path> [--folder <id>]")
	}
	filePath := args[0]
	folderID := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--folder" {
			if i+1 < len(args) {
				folderID = args[i+1]
				i++
			}
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fail(fmt.Sprintf("read file: %v", err))
	}

	fileName := filepath.Base(filePath)
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	b64 := base64.StdEncoding.EncodeToString(data)
	resp, err := apiCall("file.upload", map[string]interface{}{
		"file_name":   fileName,
		"mime_type":   mimeType,
		"data_b64":    b64,
		"total_size":  len(data),
		"folder_id":   folderID,
	})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		outputPrettyEntry(resp)
	} else {
		if !silent {
			fmt.Printf("File %q uploaded.\n", fileName)
		}
		outputResp(resp)
	}
	return 0
}

func cmdFileDownload(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim files download <manifest-block-id> [--output <path>]")
	}
	manifestBlockID := args[0]
	outputPath := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--output" || args[i] == "-o" {
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			}
		}
	}

	resp, err := apiCall("file.download", map[string]string{
		"manifest_block_id": manifestBlockID,
	})
	if err != nil {
		return fail(err.Error())
	}

	var result struct {
		FileName string `json:"file_name"`
		DataB64  string `json:"data_b64"`
		MimeType string `json:"mime_type"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		outputResp(resp)
		return 0
	}

	decoded, err := base64.StdEncoding.DecodeString(result.DataB64)
	if err != nil {
		return fail(fmt.Sprintf("decode file data: %v", err))
	}

	if outputPath == "" {
		outputPath = result.FileName
	}
	if err := os.WriteFile(outputPath, decoded, 0644); err != nil {
		return fail(fmt.Sprintf("write file: %v", err))
	}

	if !silent {
		fmt.Printf("Downloaded %q (%d bytes)\n", outputPath, len(decoded))
	}
	if !pretty {
		output(map[string]string{"downloaded": outputPath, "size": fmt.Sprintf("%d", len(decoded))})
	}
	return 0
}

func cmdFileListFolder(args []string) int {
	folderID := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--folder-id" {
			if i+1 < len(args) {
				folderID = args[i+1]
				i++
			}
		}
	}

	resp, err := apiCall("folder.list", map[string]string{"parent_id": folderID})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		outputPrettyFolderList(resp)
	} else {
		outputResp(resp)
	}
	return 0
}

func cmdFileCreateFolder(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim files create-folder <name> [--parent-id <id>]")
	}
	name := args[0]
	parentID := ""
	for i := 1; i < len(args); i++ {
		if args[i] == "--parent-id" {
			if i+1 < len(args) {
				parentID = args[i+1]
				i++
			}
		}
	}

	resp, err := apiCall("folder.create", map[string]string{
		"name":      name,
		"parent_id": parentID,
	})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Folder %q created.\n", name)
	}
	outputResp(resp)
	return 0
}

func cmdFileRenameFolder(args []string) int {
	if len(args) < 2 {
		return fail("usage: grim files rename-folder <id> <name>")
	}

	resp, err := apiCall("folder.rename", map[string]string{
		"id":   args[0],
		"name": args[1],
	})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Folder %s renamed to %s.\n", args[0], args[1])
	}
	outputResp(resp)
	return 0
}

func cmdFileDeleteFolder(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim files delete-folder <id>")
	}

	resp, err := apiCall("folder.delete", map[string]string{"id": args[0]})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Folder %s deleted.\n", args[0])
	}
	outputResp(resp)
	return 0
}

func cmdFileMove(args []string) int {
	if len(args) < 2 {
		return fail("usage: grim files move <manifest-block-id> <folder-id>")
	}

	resp, err := apiCall("file.move", map[string]string{
		"manifest_block_id": args[0],
		"folder_id":         args[1],
	})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("File %s moved to folder %s.\n", args[0], args[1])
	}
	outputResp(resp)
	return 0
}

// ─── Workspace Commands ──────────────────────────────────────────────────────

func cmdWorkspaces(subcmd string, args []string) int {
	switch subcmd {
	case "list":
		return cmdWorkspaceList()
	case "create":
		return cmdWorkspaceCreate(args)
	case "switch":
		return cmdWorkspaceSwitch(args)
	case "rename":
		return cmdWorkspaceRename(args)
	case "delete":
		return cmdWorkspaceDelete(args)
	default:
		printWorkspacesUsage()
		return 1
	}
}

func cmdWorkspaceList() int {
	resp, err := apiCall("workspace.list", map[string]string{})
	if err != nil {
		return fail(err.Error())
	}
	outputResp(resp)
	return 0
}

func cmdWorkspaceCreate(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim workspaces create <name>")
	}

	resp, err := apiCall("workspace.create", map[string]string{"name": args[0]})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Workspace %q created.\n", args[0])
	}
	outputResp(resp)
	return 0
}

func cmdWorkspaceSwitch(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim workspaces switch <id>")
	}

	resp, err := apiCall("workspace.switch", map[string]string{"id": args[0]})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Switched to workspace %s.\n", args[0])
	}
	outputResp(resp)
	return 0
}

func cmdWorkspaceRename(args []string) int {
	if len(args) < 2 {
		return fail("usage: grim workspaces rename <id> <name>")
	}

	resp, err := apiCall("workspace.rename", map[string]string{
		"id":   args[0],
		"name": args[1],
	})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Workspace %s renamed to %s.\n", args[0], args[1])
	}
	outputResp(resp)
	return 0
}

func cmdWorkspaceDelete(args []string) int {
	if len(args) < 1 {
		return fail("usage: grim workspaces delete <id>")
	}

	resp, err := apiCall("workspace.delete", map[string]string{"id": args[0]})
	if err != nil {
		return fail(err.Error())
	}

	if !silent {
		fmt.Printf("Workspace %s deleted.\n", args[0])
	}
	outputResp(resp)
	return 0
}

// ─── Sync Commands ───────────────────────────────────────────────────────────

func cmdSync(subcmd string) int {
	switch subcmd {
	case "status":
		return cmdSyncStatus()
	case "trigger":
		return cmdSyncTrigger()
	default:
		printSyncUsage()
		return 1
	}
}

func cmdSyncStatus() int {
	resp, err := apiCall("sync.status", nil)
	if err != nil {
		return fail(err.Error())
	}
	outputResp(resp)
	return 0
}

func cmdSyncTrigger() int {
	resp, err := apiCall("sync.trigger", nil)
	if err != nil {
		return fail(err.Error())
	}
	if !silent {
		fmt.Println("Sync triggered.")
	}
	outputResp(resp)
	return 0
}

// ─── Audit Command ───────────────────────────────────────────────────────────

func cmdAudit(args []string) int {
	n := 50
	for i := 0; i < len(args); i++ {
		if args[i] == "--n" || args[i] == "-n" {
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &n)
				i++
			}
		}
	}

	resp, err := apiCall("audit.list", map[string]int{"count": n})
	if err != nil {
		return fail(err.Error())
	}

	if pretty {
		var events []map[string]interface{}
		if err := json.Unmarshal(resp.Payload, &events); err == nil {
			fmt.Printf("Audit log (last %d events):\n", n)
			for _, e := range events {
				b, _ := json.MarshalIndent(e, "  ", "  ")
				fmt.Printf("  %s\n", string(b))
			}
		} else {
			outputResp(resp)
		}
	} else {
		outputResp(resp)
	}
	return 0
}

// ─── Health Command ──────────────────────────────────────────────────────────

func cmdHealth() int {
	req, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
	if err != nil {
		return fail(fmt.Sprintf("create request: %v", err))
	}
	if token != "" {
		req.Header.Set("X-Grimlocker-Token", token)
	}

	resp, err := httpClent.Do(req)
	if err != nil {
		return fail(fmt.Sprintf("health: %v", err))
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if pretty {
		var v interface{}
		if err := json.Unmarshal(body, &v); err == nil {
			out, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(out))
		} else {
			fmt.Println(string(body))
		}
	} else if silent {
		return 0
	} else {
		fmt.Println(string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// ─── SSH Key Generation (local, no API needed) ───────────────────────────────

func cmdSSHKeyGen(args []string) {
	comment := "grim-generated"
	saveToVault := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--comment":
			if i+1 < len(args) {
				comment = args[i+1]
				i++
			}
		case "--save-to-vault":
			saveToVault = true
		}
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grim: key generation failed: %v\n", err)
		os.Exit(1)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grim: marshal public key: %v\n", err)
		os.Exit(1)
	}

	pubLine := string(ssh.MarshalAuthorizedKey(sshPub))
	pubLine = strings.TrimSuffix(pubLine, "\n")
	if comment != "" {
		pubLine = pubLine + " " + comment
	}
	pubLine = pubLine + "\n"

	fp := ssh.FingerprintSHA256(sshPub)

	privPEM, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "grim: marshal private key: %v\n", err)
		os.Exit(1)
	}
	privPEMBytes := pem.EncodeToMemory(privPEM)

	if silent {
		result := map[string]string{
			"public_key":   strings.TrimSpace(pubLine),
			"fingerprint":  fp,
			"private_key":  string(privPEMBytes),
			"comment":      comment,
		}
		out, _ := json.Marshal(result)
		fmt.Println(string(out))
		return
	}

	if pretty || !saveToVault {
		fmt.Println("=== SSH Key Generated ===")
		fmt.Printf("Comment:     %s\n", comment)
		fmt.Printf("Fingerprint: %s\n\n", fp)
		fmt.Println("Public Key:")
		fmt.Print(pubLine)
		fmt.Println("\nPrivate Key (PEM):")
		fmt.Print(string(privPEMBytes))
	}

	if saveToVault {
		publicKey := strings.TrimSpace(pubLine)
		resp, err := apiCall("entry.create", map[string]interface{}{
			"title":    comment,
			"type":     "ssh",
			"category": "SSH_KEY",
			"fields": map[string]string{
				"publicKey":   publicKey,
				"fingerprint": fp,
				"comment":     comment,
			},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "grim: save to vault failed: %v\n", err)
			os.Exit(1)
		}
		if !pretty {
			fmt.Println("\nSaved to vault.")
			outputResp(resp)
		}
	}
}

// ─── Pretty Output Helpers ───────────────────────────────────────────────────

func outputPrettyTable(resp *apiResponse, category string) {
	var entries []map[string]interface{}
	if err := json.Unmarshal(resp.Payload, &entries); err != nil {
		var wrapper struct {
			Entries  []map[string]interface{} `json:"entries"`
			Category string                   `json:"category"`
			Count    int                      `json:"count"`
		}
		if err := json.Unmarshal(resp.Payload, &wrapper); err != nil {
			outputResp(resp)
			return
		}
		entries = wrapper.Entries
	}

	title := "Entries"
	if category != "" {
		title = fmt.Sprintf("Entries (category: %s)", category)
	}
	fmt.Printf("=== %s ===\n", title)
	if len(entries) == 0 {
		fmt.Println("(none)")
		return
	}
	for _, e := range entries {
		printEntrySummary(e)
	}
}

func printEntrySummary(e map[string]interface{}) {
	id := ""
	entryTitle := ""
	cat := ""
	typ := ""
	if v, ok := e["id"].(string); ok {
		id = v
	}
	if v, ok := e["title"].(string); ok {
		entryTitle = v
	}
	if v, ok := e["category"].(string); ok {
		cat = v
	}
	if v, ok := e["type"].(string); ok {
		typ = v
	}
	if typ == "" {
		typ = cat
	}
	fmt.Printf("  [%s] %s", truncateID(id), entryTitle)
	if typ != "" {
		fmt.Printf("  (%s)", typ)
	}
	fmt.Println()
}

func outputPrettyEntry(resp *apiResponse) {
	var v interface{}
	if err := json.Unmarshal(resp.Payload, &v); err != nil {
		outputResp(resp)
		return
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}

func outputPrettyFolderList(resp *apiResponse) {
	var result struct {
		ParentID string `json:"parent_id"`
		Folders  []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			ParentID string `json:"parent_id"`
			Type     string `json:"type"`
		} `json:"folders"`
		Files []struct {
			ID              string `json:"id"`
			FileName        string `json:"file_name"`
			MIMEType        string `json:"mime_type"`
			TotalSize       int64  `json:"total_size"`
			ManifestBlockID string `json:"manifest_block_id"`
			FolderID        string `json:"folder_id"`
			Type            string `json:"type"`
		} `json:"files"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		outputResp(resp)
		return
	}

	fmt.Println("=== FileVault ===")
	if result.ParentID != "" {
		fmt.Printf("Parent folder: %s\n\n", result.ParentID)
	}
	if len(result.Folders) == 0 && len(result.Files) == 0 {
		fmt.Println("(empty)")
		return
	}
	if len(result.Folders) > 0 {
		fmt.Println("Folders:")
		for _, f := range result.Folders {
			fmt.Printf("  [%s] %s\n", truncateID(f.ID), f.Name)
		}
	}
	if len(result.Files) > 0 {
		fmt.Println("\nFiles:")
		for _, f := range result.Files {
			size := formatSize(f.TotalSize)
			fmt.Printf("  [%s] %s  %s  %s\n", truncateID(f.ManifestBlockID), f.FileName, f.MIMEType, size)
		}
	}
}

func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for nn := n / unit; nn >= unit; nn /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ─── Usage ───────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Printf(`grim — Grimlocker CLI %s

Usage:
  grim <command> [args...] [--pretty] [--silent]

Global flags:
  --pretty    Human-readable formatted output
  --silent    Suppress all output (machine-only mode)

Environment:
  GRIMLOCKER_URL    API base URL (default: http://127.0.0.1:36353)
  GRIMLOCKER_TOKEN  Authentication token

Vault commands:
  grim unlock --password <password>      Unlock the vault
  grim lock                              Lock the vault
  grim status                            Show vault status
  grim health                            Check daemon health
  grim version                           Print version
  grim ssh-keygen [--comment <c>] [--save-to-vault]
                                         Generate Ed25519 SSH key pair

Entry commands:
  grim entries list [--category PASSWORD|SSH_KEY|CERTIFICATE|FILE_VAULT]
  grim entries get <id>
  grim entries create password --title <t> --username <u> --password <p> [--url <u>] [--notes <n>]
  grim entries create ssh-key --title <t> --public-key <k> [--username <u>] [--passphrase <p>]
  grim entries create certificate --title <t> --domain <d> --cert <c> --private-key <k>
  grim entries update <id> --field key=value [--field key2=value2...]
  grim entries delete <id>
  grim entries search <query> [--category <c>]

File commands:
  grim files upload <path> [--folder <id>]
  grim files download <manifest-block-id> [--output <path>]
  grim files list-folder [--folder-id <id>]
  grim files create-folder <name> [--parent-id <id>]
  grim files rename-folder <id> <name>
  grim files delete-folder <id>
  grim files move <manifest-block-id> <folder-id>

Workspace commands:
  grim workspaces list
  grim workspaces create <name>
  grim workspaces switch <id>
  grim workspaces rename <id> <name>
  grim workspaces delete <id>

Sync commands:
  grim sync status
  grim sync trigger

Audit:
  grim audit [--n 50]
`, version)
}

func printEntriesUsage() {
	fmt.Println("Usage: grim entries <subcommand> [args...]")
	fmt.Println()
	fmt.Println("Subcommands: list, get, create, update, delete, search")
	fmt.Println("Run 'grim help' for full usage.")
}

func printFilesUsage() {
	fmt.Println("Usage: grim files <subcommand> [args...]")
	fmt.Println()
	fmt.Println("Subcommands: upload, download, list-folder, create-folder, rename-folder, delete-folder, move")
	fmt.Println("Run 'grim help' for full usage.")
}

func printWorkspacesUsage() {
	fmt.Println("Usage: grim workspaces <subcommand> [args...]")
	fmt.Println()
	fmt.Println("Subcommands: list, create, switch, rename, delete")
	fmt.Println("Run 'grim help' for full usage.")
}

func printSyncUsage() {
	fmt.Println("Usage: grim sync <status|trigger>")
	fmt.Println("Run 'grim help' for full usage.")
}

func printEntriesCreateUsage() {
	fmt.Println("Usage: grim entries create <type> [args...]")
	fmt.Println()
	fmt.Println("Types: password, ssh-key, certificate")
	fmt.Println("Run 'grim help' for full usage.")
}
