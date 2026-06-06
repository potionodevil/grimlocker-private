// Package engine ist das Domain-Core von Grimlocker.
//
// Es enthält NUR reine Data-Logik: Kryptografie, Storage-Abstraktionen,
// Security-Primitives, das GQL-Protokoll, den Kernel-Event-Bus, Error-Types
// und kryptografische Tools. Es hat NULL Wissen von:
//
//   - Passwörtern (werden als pre-gehashte []byte über den Adapter empfangen)
//   - OS-File-I/O (abstrahiert hinter FileSystem-Interface)
//   - Netzwerk-Protokollen (HTTP, WebSocket, IPC)
//   - OS-Signalen, Process-Lifecycle oder Tauri-Integration
//
// Das daemon/-Package importiert engine/ durch diese öffentlichen Interfaces.
package engine

import (
	"github.com/grimlocker/grimdb/engine/crypto"
	"github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/gql"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
)

// ── Crypto ────────────────────────────────────────────────────────────────────

type (
	CryptoProvider = crypto.Provider
	KDFOptions     = crypto.KDFOptions
	PQCProvider    = crypto.PQCProvider
)

// ── Storage ───────────────────────────────────────────────────────────────────

type (
	BlockStore           = storage.BlockStore
	BlockStoreV2         = storage.BlockStoreV2
	WriteTransaction     = storage.WriteTransaction
	ReadTransaction      = storage.ReadTransaction
	StorageStrategy      = storage.StorageStrategy
)

// ── Security ──────────────────────────────────────────────────────────────────

type (
	SessionContext       = security.SessionContext
	LockdownManager      = security.LockdownManager
	AuditLog             = security.AuditLog
	MVKStore             = security.MVKStore
	MemoryGuard          = security.MemoryGuard
	IntrusionDetector    = security.IntrusionDetector
)

// ── GQL ───────────────────────────────────────────────────────────────────────

type (
	SessionInfo = gql.SessionInfo
	GQLQuery    = gql.GQLQuery
	GQLEntry    = gql.GQLEntry
	GQLResult   = gql.GQLResult
)

// ── Kernel ────────────────────────────────────────────────────────────────────

type (
	Dispatcher    = kernel.Dispatcher
	Module        = kernel.Module
	Event         = kernel.Event
	Handler       = kernel.Handler
	EventType     = kernel.EventType
	ModuleFactory = kernel.ModuleFactory
)

// ── Errors ────────────────────────────────────────────────────────────────────

type (
	GrimlockError    = errors.GrimlockError
	StructuredLogger = errors.StructuredLogger
)
