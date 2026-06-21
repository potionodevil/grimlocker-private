// Package kernel (event.go) definiert den Event-Type und alle EventType-Konstanten
// für den Grimlocker-Daemon. Jede Inter-Module-Nachricht ist ein Event.
//
// EventType-Naming-Convention: CHANNEL.ACTION (z.B. "CRYPTO.ENCRYPT").
// Das Channel-Präfix (alles vor ".") wird vom Bus genutzt, um Events an die
// richtige Module.Handle-Implementierung zu routen.
//
// Neues Event hinzufügen:
//  1. Konstante hier definieren (z.B. EvFooBar EventType = "FOO.BAR").
//  2. Handler im besitzenden Module via buildHandlers() / buildRegistry() registrieren.
//  3. JSON-Payload-Schema in einem Kommentar neben der Konstante dokumentieren.
package kernel

// EventType ist die Channel-Adresse eines Events. Der Prefix vor "." ist der
// Channel des besitzenden Modules (z.B. "CRYPTO" für alle CRYPTO.*-Events).
type EventType string

const (
	// AUTH-Channel — owned by security.Module
	EvAuthSetup     EventType = "AUTH.SETUP"
	EvAuthUnlock    EventType = "AUTH.UNLOCK"
	EvAuthResult    EventType = "AUTH.RESULT"
	EvAuthLockdown  EventType = "AUTH.LOCKDOWN"
	EvAuthLogout    EventType = "AUTH.LOGOUT"
	EvAuthStatus    EventType = "AUTH.STATUS"
	EvAuthInitReady EventType = "AUTH.INIT_READY"
	EvAuthKeyReady  EventType = "AUTH.KEY_READY"
	EvAuthReady     EventType = "AUTH.READY"
	EvAuthGetHandle EventType = "AUTH.GET_HANDLE"

	// CRYPTO-Channel — owned by crypto.Module
	EvCryptoEncrypt EventType = "CRYPTO.ENCRYPT"
	EvCryptoDecrypt EventType = "CRYPTO.DECRYPT"
	EvCryptoDerive  EventType = "CRYPTO.DERIVE_KEY"
	EvCryptoShred   EventType = "CRYPTO.SHRED"
	EvCryptoResult  EventType = "CRYPTO.RESULT"

	// STORAGE-Channel — owned by storage adapter
	EvStorageWrite          EventType = "STORAGE.WRITE"
	EvStorageRead           EventType = "STORAGE.READ"
	EvStorageDelete         EventType = "STORAGE.DELETE"
	EvStorageList           EventType = "STORAGE.LIST"
	EvStorageResult         EventType = "STORAGE.RESULT"
	EvStorageIngestProgress EventType = "STORAGE.INGEST_PROGRESS"
	EvStorageVFSMount       EventType = "STORAGE.VFS_MOUNT"
	EvStorageReady          EventType = "STORAGE.READY"

	// ENTRY-Channel — owned by entry handler module
	EvEntryCreate EventType = "ENTRY.CREATE"
	EvEntryRead   EventType = "ENTRY.READ"
	EvEntryUpdate EventType = "ENTRY.UPDATE"
	EvEntryDelete EventType = "ENTRY.DELETE"
	EvEntryIngest EventType = "ENTRY.INGEST"
	EvEntryResult EventType = "ENTRY.RESULT"
	EvEntryQuery  EventType = "ENTRY.QUERY"

	// TOOL-Channel — owned by tools module
	EvToolSSHGen EventType = "TOOL.SSH_GEN"
	EvToolResult EventType = "TOOL.RESULT"

	// SECURITY-Channel — owned by security.Module
	EvSecMemLock  EventType = "SECURITY.MEM_LOCK"
	EvSecZeroize  EventType = "SECURITY.ZEROIZE"
	EvSecAudit    EventType = "SECURITY.AUDIT"
	EvSecPanic    EventType = "SECURITY.PANIC"
	EvSecLockdown EventType = "SECURITY.LOCKDOWN"

	// SYNC-Channel — available to SDK plugins + Local Network Sync
	EvSyncBegin    EventType = "SYNC.BEGIN"
	EvSyncComplete EventType = "SYNC.COMPLETE"
	EvSyncDiscover EventType = "SYNC.DISCOVER"
	EvSyncPair     EventType = "SYNC.PAIR"
	EvSyncPull     EventType = "SYNC.PULL"
	EvSyncPushVer  EventType = "SYNC.PUSH_VERSION"
	EvSyncConflict EventType = "SYNC.CONFLICT"

	// BIOMETRIC-Channel — used by hardware sensor plugins
	EvBiometricAuthenticate EventType = "BIOMETRIC.AUTHENTICATE"
	EvBiometricResult       EventType = "BIOMETRIC.RESULT"

	// INTEGRITY-Channel — used by the binary integrity monitor
	EvIntegrityCheck     EventType = "INTEGRITY.CHECK"
	EvIntegrityViolation EventType = "INTEGRITY.VIOLATION"

	// WORKSPACE-Channel — multi-tenant vault management
	EvWorkspaceCreate EventType = "WORKSPACE.CREATE"
	EvWorkspaceSwitch EventType = "WORKSPACE.SWITCH"
	EvWorkspaceDelete EventType = "WORKSPACE.DELETE"
	EvWorkspaceResult EventType = "WORKSPACE.RESULT"

	// KERNEL-Channel — handshake & status reporting
	EvKernelStatus      EventType = "KERNEL.STATUS"
	EvKernelStateReport EventType = "KERNEL.STATE_REPORT"
	EvKernelStateMirror EventType = "KERNEL.STATE_MIRROR"

	// RECONNECT-Channel — UI re-attach protocol (Phase 3)
	EvReconnectResume EventType = "RECONNECT.RESUME"
	EvReconnectSync   EventType = "RECONNECT.SYNC"

	// GQL-Channel — binary protocol (Phase 4)
	EvGQLQuery  EventType = "GQL.QUERY"
	EvGQLResult EventType = "GQL.RESULT"

	// SYSTEM-Channel — errors, health, telemetry
	EvSystemError       EventType = "SYSTEM.ERROR"
	EvSystemHealthCheck EventType = "SYSTEM.HEALTH_CHECK"
	EvSystemLog         EventType = "SYSTEM.LOG"

	// BACKUP-Channel — air-gap export and two-phase import
	// Payload schemas are defined in engine/backup/types.go.
	EvBackupExport          EventType = "BACKUP.EXPORT"
	EvBackupPeek            EventType = "BACKUP.PEEK"
	EvBackupAuthorize       EventType = "BACKUP.AUTHORIZE"
	EvBackupChecksum        EventType = "BACKUP.CHECKSUM"
	EvBackupResult          EventType = "BACKUP.RESULT"
	EvBackupChecksumComplete EventType = "BACKUP.CHECKSUM_COMPLETE"
)

// Event ist die Kommunikationseinheit zwischen allen Modulen. Payloads sind JSON.
// Kein Modul darf die Funktionen eines anderen Moduls direkt aufrufen — die
// Kommunikation MUSS ausschließlich über Events auf dem Bus laufen.
type Event struct {
	// ID ist eine UUID v4 zum Korrelieren von Requests und Responses.
	ID string `json:"id"`

	Type EventType `json:"type"`

	// Payload ist JSON-encoded; das Schema ist pro EventType definiert.
	Payload []byte `json:"payload,omitempty"`

	// ReplyTo enthält die Event.ID des Ursprungs, wenn dieses Event eine Response ist.
	ReplyTo string `json:"reply_to,omitempty"`

	// Origin ist die Modul-ID, die dieses Event dispatched hat.
	Origin string `json:"origin,omitempty"`

	// Timestamp in Unix-Nanoseconds.
	Timestamp int64 `json:"timestamp"`

	// TTL wird pro Hop dekrementiert; der Bus droppt Events bei 0 (Cycle-Schutz).
	TTL int `json:"ttl"`
}

// Channel extrahiert das Routing-Präfix aus einem EventType (z.B. "CRYPTO" aus "CRYPTO.ENCRYPT").
func (et EventType) Channel() string {
	s := string(et)
	for i, c := range s {
		if c == '.' {
			return s[:i]
		}
	}
	return s
}
