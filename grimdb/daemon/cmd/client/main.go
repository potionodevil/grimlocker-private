// grimlocker-client is the universal Grimlocker CLI.
//
// It auto-detects the tier from environment variables:
//   - GRIMLOCKER_DAEMON_ADDR set → Enterprise (remote mTLS)
//   - GRIMLOCKER_IPC set (ws://...) → Single-User (local IPC)
//
// Usage:
//
//	grimlocker unlock <password|jwt-token>
//	grimlocker get <entry-id>
//	grimlocker set <entry-id> <value> [category]
//	grimlocker list [category]
//	grimlocker lock
//	grimlocker audit [count]
//	grimlocker health
package main

import (
	"fmt"
	"os"
	"strconv"
)

const version = "omega-enterprise-2026-05-30"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Handle commands that don't require a daemon connection.
	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("grimlocker %s\n", version)
		return
	case "help", "--help", "-h":
		printUsage()
		return
	}

	conn, err := connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to daemon: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := dispatch(conn, cmd, args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// connect creates the appropriate daemon connection based on environment.
func connect() (daemonConn, error) {
	// Enterprise: GRIMLOCKER_DAEMON_ADDR=host:9443
	if addr := os.Getenv("GRIMLOCKER_DAEMON_ADDR"); addr != "" {
		certPath := envOrDefault("GRIMLOCKER_CLIENT_CERT", "client.crt")
		keyPath := envOrDefault("GRIMLOCKER_CLIENT_KEY", "client.key")
		caPath := envOrDefault("GRIMLOCKER_CA_CERT", "ca.crt")
		return newRemoteConn(addr, certPath, keyPath, caPath)
	}

	// Single-User: GRIMLOCKER_IPC=ws://127.0.0.1:PORT/ws
	ipcURL := os.Getenv("GRIMLOCKER_IPC")
	if ipcURL == "" {
		return nil, fmt.Errorf(
			"no daemon address configured.\n" +
				"  Single-user: set GRIMLOCKER_IPC=ws://127.0.0.1:PORT/ws\n" +
				"  Enterprise:  set GRIMLOCKER_DAEMON_ADDR=host:9443")
	}
	token := os.Getenv("GRIMLOCKER_TOKEN")
	return newLocalConn(parseWSToHTTP(ipcURL), token), nil
}

// dispatch routes the CLI command to the appropriate handler.
func dispatch(conn daemonConn, cmd string, args []string) error {
	switch cmd {
	case "init":
		if len(args) < 1 {
			return fmt.Errorf("usage: grimlocker init <password>")
		}
		return cmdInit(conn, args[0])

	case "unlock":
		if len(args) < 1 {
			return fmt.Errorf("usage: grimlocker unlock <password|token>")
		}
		return cmdUnlock(conn, args[0])

	case "get":
		if len(args) < 1 {
			return fmt.Errorf("usage: grimlocker get <entry-id>")
		}
		return cmdGet(conn, args[0])

	case "set", "create":
		if len(args) < 2 {
			return fmt.Errorf("usage: grimlocker set <entry-id> <value> [category]")
		}
		category := ""
		if len(args) >= 3 {
			category = args[2]
		}
		return cmdSet(conn, args[0], args[1], category)

	case "update":
		if len(args) < 2 {
			return fmt.Errorf("usage: grimlocker update <entry-id> <value> [category]")
		}
		category := ""
		if len(args) >= 3 {
			category = args[2]
		}
		return cmdUpdate(conn, args[0], args[1], category)

	case "delete", "rm":
		if len(args) < 1 {
			return fmt.Errorf("usage: grimlocker delete <entry-id>")
		}
		return cmdDelete(conn, args[0])

	case "list", "ls":
		category := ""
		if len(args) >= 1 {
			category = args[0]
		}
		return cmdList(conn, category)

	case "lock":
		return cmdLock(conn)

	case "status":
		return cmdStatus(conn)

	case "health":
		return cmdHealth(conn)

	case "audit":
		n := 20
		if len(args) >= 1 {
			if v, err := strconv.Atoi(args[0]); err == nil {
				n = v
			}
		}
		return cmdAudit(conn, n)

	default:
		return fmt.Errorf("unknown command %q — run 'grimlocker help'", cmd)
	}
}

func printUsage() {
	fmt.Printf(`Grimlocker CLI %s

Usage:
  grimlocker <command> [args...]

Commands:
  init   <password>               Initialize a new vault (first-time setup)
  unlock <password|token>         Authenticate and unlock the vault
  get    <entry-id>               Retrieve an entry by ID
  set    <entry-id> <val> [cat]   Create an entry (aliases: create)
  update <entry-id> <val> [cat]   Update an existing entry
  delete <entry-id>               Delete an entry (aliases: rm)
  list   [category]               List entries, optional category filter (aliases: ls)
  lock                            Lock the vault (ends session)
  status                          Show vault status (initialized, locked)
  audit  [count]                  Show recent audit log entries (default: 20)
  health                          Check daemon health status
  version                         Print CLI version

Environment:
  GRIMLOCKER_DAEMON_ADDR  Enterprise daemon address (host:9443) → mTLS mode
  GRIMLOCKER_IPC          Single-user daemon IPC URL (ws://127.0.0.1:PORT/ws)
  GRIMLOCKER_TOKEN        Authentication token for single-user mode
  GRIMLOCKER_CLIENT_CERT  Client TLS certificate path (enterprise, default: client.crt)
  GRIMLOCKER_CLIENT_KEY   Client TLS private key path  (enterprise, default: client.key)
  GRIMLOCKER_CA_CERT      CA certificate path           (enterprise, default: ca.crt)
`, version)
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
