//go:build enterprise

package main

// daemonTier gibt "enterprise" zurück — wird im /health-Endpoint als "tier"-Feld verwendet.
func daemonTier() string { return "enterprise" }

// sessionUserRole leitet die Rolle des aktuell authentifizierten Users ab.
// Stub: gibt "admin" zurück bis das OIDC-RBAC-System verdrahtet ist.
// TODO: OIDC JWT Claims auslesen und Rolle daraus ableiten (sub → "admin"|"user").
func sessionUserRole(_ string) string { return "admin" }
