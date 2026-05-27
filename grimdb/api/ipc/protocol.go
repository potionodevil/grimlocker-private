package ipc

// Message type constants. Wire format: 4-byte big-endian length prefix, then
// 1-byte type, then N-byte payload. Unchanged from grimdb-go to preserve
// compatibility with the existing Rust CLI and Tauri frontend.
const (
	MsgGetHeader          byte = 0x01
	MsgHeader             byte = 0x02
	MsgGetCiphertext      byte = 0x03
	MsgCiphertext         byte = 0x04
	MsgUpdateHeader       byte = 0x05
	MsgUpdateCiphertext   byte = 0x06
	MsgTriggerWipe        byte = 0x07
	MsgAck                byte = 0x08
	MsgError              byte = 0x09
	MsgPanicWipe          byte = 0x0A
	MsgGenerateMatrix     byte = 0x0B
	MsgProgress           byte = 0x0C
	MsgGenerationResult   byte = 0x0D
	MsgZeroizeConfirm     byte = 0x0E
	MsgInitializeVault    byte = 0x0F
	MsgUnlockVault        byte = 0x10
	MsgSaveEntry          byte = 0x11
	MsgRecoveryPhrase     byte = 0x12
	MsgUnlockResult       byte = 0x13
	MsgCheckVaultStatus   byte = 0x14
	MsgListEntries        byte = 0x15
	MsgGetEntry           byte = 0x16
	MsgDeleteEntry        byte = 0x17
	MsgEntriesResult      byte = 0x18
	MsgEntryData          byte = 0x19
	MsgResetVault         byte = 0x1A
	MsgLogBroadcast       byte = 0x1B
	MsgEntryCreate        byte = 0x1C
	MsgEntryResult        byte = 0x1D
	MsgEntryUpdate        byte = 0x1E
	MsgEntryDelete        byte = 0x1F
	MsgFileIngestBegin    byte = 0x20
	MsgFileChunk          byte = 0x21
	MsgFileIngestEnd      byte = 0x22
	MsgIngestProgress     byte = 0x23
	MsgGetRecoveryPhrase  byte = 0x24 // client → server: request recovery phrase
	MsgRecoveryPhraseData byte = 0x25 // server → client: encrypted recovery phrase
	MsgPanicWipeRequest   byte = 0x26 // client → server: request complete wipe (destructive)
	MsgWorkspaceList      byte = 0x27 // client → server: list all workspaces
	MsgWorkspaceCreate    byte = 0x28 // client → server: JSON {name}
	MsgWorkspaceSwitch    byte = 0x29 // client → server: JSON {id}
	MsgWorkspaceDelete    byte = 0x2A // client → server: JSON {id}
	MsgWorkspacesResult   byte = 0x2B // server → client: JSON []Workspace

	// Handshake protocol
	MsgInitReady         byte = 0x2C // server → client: INIT.READY
	MsgAuthTokenSubmit   byte = 0x2D // client → server: AUTH.TOKEN_SUBMIT
	MsgKernelStateReady  byte = 0x2E // server → client: KERNEL.STATE_READY
	MsgSystemHeartbeat   byte = 0x2F // server → client: SYSTEM.HEARTBEAT
	MsgSystemError       byte = 0x30 // server → client: SYSTEM.ERROR
	MsgSystemLog         byte = 0x31 // server → client: SYSTEM.LOG
	MsgSystemHealthCheck byte = 0x32 // bidirectional: SYSTEM.HEALTH_CHECK

	// Session-key encrypted data (SKE) — the daemon encrypts sensitive entry
	// data with a per-session ChaCha20-Poly1305 key before sending it over
	// the WebSocket. The frontend holds the key in RAM and decrypts locally
	// only when the user explicitly clicks "Reveal".
	MsgDecryptEntry  byte = 0x33 // client → server: JSON {id: "..."}
	MsgDecryptedData byte = 0x34 // server → client: SKE-encrypted entry JSON (base64)

	CookieSize   = 32
	UnixSockPath = "/tmp/grimlocker.sock"
	WinPipePath  = `\\.\\pipe\\grimlocker`
)
