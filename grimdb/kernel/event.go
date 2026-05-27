package kernel

// EventType is the channel address of an event. The prefix before "." is the
// owning module's channel (e.g. "CRYPTO" for all CRYPTO.* events).
type EventType string

const (
	// AUTH channel — owned by security.Module
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

	// CRYPTO channel — owned by crypto.Module
	EvCryptoEncrypt EventType = "CRYPTO.ENCRYPT"
	EvCryptoDecrypt EventType = "CRYPTO.DECRYPT"
	EvCryptoDerive  EventType = "CRYPTO.DERIVE_KEY"
	EvCryptoShred   EventType = "CRYPTO.SHRED"
	EvCryptoResult  EventType = "CRYPTO.RESULT"

	// STORAGE channel — owned by storage adapter
	EvStorageWrite          EventType = "STORAGE.WRITE"
	EvStorageRead           EventType = "STORAGE.READ"
	EvStorageDelete         EventType = "STORAGE.DELETE"
	EvStorageList           EventType = "STORAGE.LIST"
	EvStorageResult         EventType = "STORAGE.RESULT"
	EvStorageIngestProgress EventType = "STORAGE.INGEST_PROGRESS"
	EvStorageVFSMount       EventType = "STORAGE.VFS_MOUNT"
	EvStorageReady          EventType = "STORAGE.READY"

	// ENTRY channel — owned by entry handler module
	EvEntryCreate EventType = "ENTRY.CREATE"
	EvEntryRead   EventType = "ENTRY.READ"
	EvEntryUpdate EventType = "ENTRY.UPDATE"
	EvEntryDelete EventType = "ENTRY.DELETE"
	EvEntryIngest EventType = "ENTRY.INGEST"
	EvEntryResult EventType = "ENTRY.RESULT"

	// SECURITY channel — owned by security.Module
	EvSecMemLock  EventType = "SECURITY.MEM_LOCK"
	EvSecZeroize  EventType = "SECURITY.ZEROIZE"
	EvSecAudit    EventType = "SECURITY.AUDIT"
	EvSecPanic    EventType = "SECURITY.PANIC"
	EvSecLockdown EventType = "SECURITY.LOCKDOWN"

	// SYNC channel — available to SDK plugins
	EvSyncBegin    EventType = "SYNC.BEGIN"
	EvSyncComplete EventType = "SYNC.COMPLETE"

	// BIOMETRIC channel — used by hardware sensor plugins
	EvBiometricAuthenticate EventType = "BIOMETRIC.AUTHENTICATE"
	EvBiometricResult       EventType = "BIOMETRIC.RESULT"

	// INTEGRITY channel — used by the binary integrity monitor
	EvIntegrityCheck     EventType = "INTEGRITY.CHECK"
	EvIntegrityViolation EventType = "INTEGRITY.VIOLATION"

	// WORKSPACE channel — multi-tenant vault management
	EvWorkspaceCreate EventType = "WORKSPACE.CREATE"
	EvWorkspaceSwitch EventType = "WORKSPACE.SWITCH"
	EvWorkspaceDelete EventType = "WORKSPACE.DELETE"
	EvWorkspaceResult EventType = "WORKSPACE.RESULT"

	// KERNEL channel — handshake & status reporting
	EvKernelStatus      EventType = "KERNEL.STATUS"
	EvKernelStateReport EventType = "KERNEL.STATE_REPORT"

	// SYSTEM channel — errors, health, telemetry
	EvSystemError       EventType = "SYSTEM.ERROR"
	EvSystemHealthCheck EventType = "SYSTEM.HEALTH_CHECK"
	EvSystemLog         EventType = "SYSTEM.LOG"
)

// Event is the unit of communication between all modules. Payloads are JSON.
// No module may call another module's functions directly; all inter-module
// communication MUST go through an Event dispatched on the bus.
type Event struct {
	// ID is a UUID v4 used to correlate requests and responses.
	ID string `json:"id"`

	Type EventType `json:"type"`

	// Payload is JSON-encoded data whose schema is defined per EventType.
	Payload []byte `json:"payload,omitempty"`

	// ReplyTo contains the originating Event.ID when this event is a response.
	ReplyTo string `json:"reply_to,omitempty"`

	// Origin is the module ID that dispatched this event.
	Origin string `json:"origin,omitempty"`

	// Timestamp is Unix nanoseconds.
	Timestamp int64 `json:"timestamp"`

	// TTL is decremented on each hop; the bus drops events at 0 to break cycles.
	TTL int `json:"ttl"`
}

// Channel extracts the routing prefix from an EventType (e.g. "CRYPTO" from "CRYPTO.ENCRYPT").
func (et EventType) Channel() string {
	s := string(et)
	for i, c := range s {
		if c == '.' {
			return s[:i]
		}
	}
	return s
}
