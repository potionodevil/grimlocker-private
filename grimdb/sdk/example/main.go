// SDK usage example — connect to a running Grimlocker daemon and exercise
// the high-level operations API.
//
// Usage:
//
//	go run ./sdk/example/main.go -addr ws://127.0.0.1:41753/ws -token <TOKEN>
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/grimlocker/grimdb/sdk"
)

func main() {
	addr := flag.String("addr", "ws://127.0.0.1:41753/ws", "daemon WebSocket address")
	token := flag.String("token", "", "session token (from GRIMLOCKER_TOKEN env or daemon stdout)")
	flag.Parse()

	if *token == "" {
		log.Fatal("--token is required")
	}

	endpoint := fmt.Sprintf("%s?token=%s", *addr, *token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := sdk.DialGQL(ctx, endpoint)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer client.Close()

	fmt.Println("=== Grimlocker SDK Example ===")

	// List all entries
	entries, err := client.ListEntries(ctx, "default")
	if err != nil {
		log.Fatalf("list entries: %v", err)
	}
	fmt.Printf("Total entries: %d\n", len(entries))
	for _, e := range entries {
		fmt.Printf("  [%s] %s — %s\n", e.Category, e.ID[:8], e.Title)
	}

	// Create a password entry
	id, err := client.CreatePassword(ctx, "default", &sdk.PasswordEntry{
		Title:    "Example Login",
		Username: "alice@example.com",
		Password: "s3cr3t!",
		URL:      "https://example.com",
		Notes:    "Created by SDK example",
	})
	if err != nil {
		log.Fatalf("create password: %v", err)
	}
	fmt.Printf("\nCreated password entry: %s\n", id)

	// List passwords
	passwords, err := client.ListPasswords(ctx, "default")
	if err != nil {
		log.Fatalf("list passwords: %v", err)
	}
	fmt.Printf("Password entries: %d\n", len(passwords))
	for _, p := range passwords {
		fmt.Printf("  %s — %s (%s)\n", p.ID[:8], p.Title, p.Username)
	}

	// Create an SSH key entry
	sshID, err := client.CreateSSHKey(ctx, "default", &sdk.SSHKeyEntry{
		Title:     "Dev Server Key",
		PublicKey: "ssh-ed25519 AAAAC3Nz... alice@dev",
		Comment:   "alice@dev",
		Algorithm: "ed25519",
	})
	if err != nil {
		log.Fatalf("create ssh key: %v", err)
	}
	fmt.Printf("Created SSH key entry: %s\n", sshID)

	// Clean up example entries
	if err := client.DeleteEntry(ctx, "default", id); err != nil {
		log.Printf("delete password: %v", err)
	}
	if err := client.DeleteEntry(ctx, "default", sshID); err != nil {
		log.Printf("delete ssh key: %v", err)
	}

	fmt.Println("\nDone.")
}
