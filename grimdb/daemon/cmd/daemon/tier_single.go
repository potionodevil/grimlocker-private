//go:build !enterprise

package main

// daemonTier gibt "single" zurück — wird im /health-Endpoint als "tier"-Feld verwendet.
func daemonTier() string { return "single" }

// sessionUserRole gibt für Single-User immer "admin" zurück.
// Beim Enterprise-Build wird diese Funktion durch tier_enterprise.go ersetzt.
func sessionUserRole(_ string) string { return "admin" }
