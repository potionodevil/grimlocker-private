package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// daemonConn is the interface implemented by both localConn and remoteConn.
type daemonConn interface {
	// Send dispatches an action to the daemon and returns the response payload.
	// action is a REST API action name (e.g. "vault.unlock", "entry.create").
	Send(action string, payload []byte) ([]byte, error)
	Health() ([]byte, error)
	Close() error
}

// cmdUnlock sends vault.unlock with the given credential.
// credential is accepted as both password (single-user) and JWT token (enterprise).
func cmdUnlock(conn daemonConn, credential string) error {
	appDir := os.Getenv("GRIMLOCKER_APP_DIR")
	payload, _ := json.Marshal(map[string]string{
		"password": credential,
		"token":    credential,
		"app_dir":  appDir,
	})
	resp, err := conn.Send("vault.unlock", payload)
	if err != nil {
		return err
	}
	// API response: {"ok":true,"payload":{"success":true/false,"reason":"..."}}
	inner, err := unwrapPayload(resp)
	if err != nil {
		return err
	}
	if success, _ := inner["success"].(bool); !success {
		reason, _ := inner["reason"].(string)
		if reason == "" {
			reason = "authentication failed"
		}
		return fmt.Errorf("unlock failed: %s", reason)
	}
	fmt.Println("✓ Vault unlocked.")
	return nil
}

// unwrapPayload extracts the inner {"payload":{...}} from an API response.
func unwrapPayload(resp []byte) (map[string]interface{}, error) {
	var outer map[string]interface{}
	if err := json.Unmarshal(resp, &outer); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	// If "ok" is false at the outer level, return the error.
	if ok, _ := outer["ok"].(bool); !ok {
		errMsg, _ := outer["error"].(string)
		return nil, fmt.Errorf("daemon error: %s", errMsg)
	}
	// Inner payload might be a map or nil.
	if p, ok := outer["payload"].(map[string]interface{}); ok {
		return p, nil
	}
	// Payload might be directly the result (some handlers return flat maps).
	return outer, nil
}

// cmdInit initializes a new vault via the /init HTTP endpoint.
// Requires a localConn (the /init route is on the same mux as /api/v1).
func cmdInit(conn daemonConn, password string) error {
	lc, ok := conn.(*localConn)
	if !ok {
		return fmt.Errorf("vault init is only supported in single-user mode")
	}
	body, _ := json.Marshal(map[string]string{"password": password})
	resp, err := lc.postRaw("/init", body)
	if err != nil {
		return fmt.Errorf("init failed: %w", err)
	}
	var result map[string]string
	if err := json.Unmarshal(resp, &result); err != nil {
		return printFormatted(resp)
	}
	if errMsg := result["error"]; errMsg != "" {
		return fmt.Errorf("init failed: %s", errMsg)
	}
	phrase := result["recovery_phrase"]
	fmt.Println("✓ Vault initialized.")
	fmt.Println("")
	fmt.Println("  RECOVERY PHRASE (save this securely — it cannot be recovered):")
	fmt.Println("  " + phrase)
	fmt.Println("")
	return nil
}

// cmdGet retrieves a vault entry by ID or title.
func cmdGet(conn daemonConn, id string) error {
	resolvedID, err := resolveEntryID(conn, id)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"id": resolvedID})
	resp, err := conn.Send("entry.read", payload)
	if err != nil {
		return err
	}
	return printFormatted(resp)
}


// cmdSet creates a new vault entry.
func cmdSet(conn daemonConn, id, value, category string) error {
	if category == "" {
		category = "password"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"id":       id,
		"title":    id,
		"category": category,
		"fields":   map[string]string{"value": value},
	})
	resp, err := conn.Send("entry.create", payload)
	if err != nil {
		return err
	}
	if _, err := unwrapPayload(resp); err != nil {
		return err
	}
	fmt.Printf("✓ Entry %q stored.\n", id)
	return nil
}

// cmdUpdate updates an existing vault entry.
func cmdUpdate(conn daemonConn, id, value, category string) error {
	if category == "" {
		category = "password"
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"id":       id,
		"title":    id,
		"category": category,
		"fields":   map[string]string{"value": value},
	})
	resp, err := conn.Send("entry.update", payload)
	if err != nil {
		return err
	}
	if _, err := unwrapPayload(resp); err != nil {
		return err
	}
	fmt.Printf("✓ Entry %q updated.\n", id)
	return nil
}

// cmdDelete removes a vault entry by ID or title.
func cmdDelete(conn daemonConn, id string) error {
	// Resolve title → ID if necessary.
	resolvedID, err := resolveEntryID(conn, id)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]string{"id": resolvedID})
	resp, err := conn.Send("entry.delete", payload)
	if err != nil {
		return err
	}
	if _, err := unwrapPayload(resp); err != nil {
		return err
	}
	fmt.Printf("✓ Entry %q deleted.\n", id)
	return nil
}

// resolveEntryID returns the UUID for an entry given either a UUID or a title.
func resolveEntryID(conn daemonConn, idOrTitle string) (string, error) {
	// Try direct lookup first.
	getPayload, _ := json.Marshal(map[string]string{"id": idOrTitle})
	getResp, err := conn.Send("entry.read", getPayload)
	if err == nil {
		inner, err2 := unwrapPayload(getResp)
		if err2 == nil {
			if _, hasError := inner["error"]; !hasError {
				return idOrTitle, nil // it's already a valid ID
			}
		}
	}
	// Search by title.
	listPayload, _ := json.Marshal(map[string]string{"category": ""})
	listResp, err := conn.Send("entry.query", listPayload)
	if err != nil {
		return "", err
	}
	listInner, err := unwrapPayload(listResp)
	if err != nil {
		return "", err
	}
	rawEntries, _ := listInner["entries"].([]interface{})
	for _, raw := range rawEntries {
		meta, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		entryID, _ := meta["id"].(string)
		if entryID == "" {
			continue
		}
		gp, _ := json.Marshal(map[string]string{"id": entryID})
		gr, err := conn.Send("entry.read", gp)
		if err != nil {
			continue
		}
		inner, err := unwrapPayload(gr)
		if err != nil {
			continue
		}
		if t, _ := inner["title"].(string); t == idOrTitle {
			return entryID, nil
		}
	}
	return "", fmt.Errorf("entry not found: %q", idOrTitle)
}

// cmdList lists vault entries, optionally filtered by category.
func cmdList(conn daemonConn, category string) error {
	payload, _ := json.Marshal(map[string]string{"category": category})
	resp, err := conn.Send("entry.query", payload)
	if err != nil {
		return err
	}
	return printFormatted(resp)
}

// cmdLock sends vault.logout to lock the vault.
func cmdLock(conn daemonConn) error {
	_, err := conn.Send("vault.logout", []byte(`{}`))
	if err != nil {
		return err
	}
	fmt.Println("✓ Vault locked.")
	return nil
}

// cmdStatus queries the vault status (initialized, locked/unlocked).
func cmdStatus(conn daemonConn) error {
	resp, err := conn.Send("vault.status", []byte(`{}`))
	if err != nil {
		return err
	}
	return printFormatted(resp)
}

// cmdHealth queries the daemon /health endpoint.
func cmdHealth(conn daemonConn) error {
	resp, err := conn.Health()
	if err != nil {
		return err
	}
	return printFormatted(resp)
}

// cmdAudit retrieves recent audit log entries.
func cmdAudit(conn daemonConn, n int) error {
	// Audit is accessible via the security module's bus event.
	// For now, print health with audit-adjacent info.
	resp, err := conn.Health()
	if err != nil {
		return err
	}
	fmt.Printf("(Audit streaming not yet wired in REST API — showing health instead)\n")
	return printFormatted(resp)
}

// printFormatted pretty-prints JSON or falls back to raw output.
func printFormatted(data []byte) error {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		fmt.Println(string(data))
		return nil
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
	return nil
}
