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

	// Category-filtered entry queries
	MsgEntryQuery       byte = 0x35 // client → server: JSON {category: "PASSWORD"|"SSH_KEY"|…}
	MsgEntryQueryResult byte = 0x36 // server → client: JSON {category, entries: [], count}

	// SSH key generation (TOOL channel)
	MsgSSHKeyGen    byte = 0x37 // client → server: JSON {comment: string, save_to_vault: bool}
	MsgSSHKeyResult byte = 0x38 // server → client: JSON {public_key, fingerprint, entry_id}

	// Reconnect / state-mirror protocol (Phase 3 — UI-agnostic lifecycle)
	MsgReconnect        byte = 0x39 // client → server: JSON {token: string} — resume session without re-auth
	MsgStateMirror      byte = 0x3A // server → client: JSON full vault state (unlocked, gate, workspace, entries, SKE handle)
	MsgSessionResumeOK  byte = 0x3B // server → client: ack that session was resumed successfully
	MsgSessionResumeErr byte = 0x3C // server → client: resume failed (session expired, vault locked, etc.)

	// GQL binary protocol (Phase 4 — GrimQueryLanguage)
	MsgGQLQuery  byte = 0x3D // client → server: GQL binary frame (injection-immune)
	MsgGQLResult byte = 0x3E // server → client: GQLResult JSON

	// Auth lifecycle
	MsgAuthLogout    byte = 0x3F // client → server: request vault lock (frontend auto-lock / user logout)
	MsgAuthLogoutAck byte = 0x40 // server → client: ack that vault has been locked

	// FileVault download (Phase 1 — streaming decrypted file retrieval)
	MsgFileDownloadRequest byte = 0x41 // client → server: JSON {manifest_block_id: string}
	MsgFileChunkData       byte = 0x42 // server → client: binary chunk (decrypted + decompressed)
	MsgFileDownloadEnd     byte = 0x43 // server → client: JSON {sha256: hex, total_size: int, file_name, mime_type}

	// Workspace rename
	MsgWorkspaceRename byte = 0x44 // client → server: JSON {id: string, name: string}

	// Enterprise security
	MsgPanicButton byte = 0x45 // client → server: JSON {passphrase: string} (Admin-only)

	// Enterprise server discovery (mDNS)
	MsgDiscoverServers byte = 0x50 // client → server: {} — scan local network
	MsgServerList      byte = 0x51 // server → client: JSON [{name, address, port, tls_required}]

	// FileVault folder system
	MsgFolderCreate byte = 0x60 // client → server: JSON {name, parent_id}
	MsgFolderList   byte = 0x61 // client → server: JSON {parent_id}
	MsgFolderRename byte = 0x62 // client → server: JSON {id, name}
	MsgFolderDelete byte = 0x63 // client → server: JSON {id}
	MsgFolderResult byte = 0x64 // server → client: JSON folder or folder contents

	// FileVault file move
	MsgFileMoveToFolder byte = 0x65 // client → server: JSON {manifest_block_id, folder_id}

	// Enterprise user management (Admin-only operations)
	MsgEnterpriseUserCreate  byte = 0x52 // client → server: JSON {username, roles[]}
	MsgEnterpriseUserList    byte = 0x53 // client → server: {}
	MsgEnterpriseUserRevoke  byte = 0x54 // client → server: JSON {user_id}
	MsgEnterpriseUserRestore byte = 0x55 // client → server: JSON {user_id}
	MsgEnterpriseUserResult  byte = 0x56 // server → client: JSON (user or list)

	// LAN Sync IPC (Single-User tier)
	MsgSyncListPeers byte = 0x70 // client → server: {} — list discovered peers + sync metadata
	MsgSyncTrigger   byte = 0x71 // client → server: {} — trigger immediate sync cycle
	MsgSyncResult    byte = 0x72 // server → client: SKE-encrypted JSON sync state

	// Audit log IPC
	MsgAuditList   byte = 0x73 // client → server: [2-byte big-endian n] — request last n audit entries
	MsgAuditResult byte = 0x74 // server → client: SKE-encrypted JSON []SecurityEvent

	CookieSize   = 32
	UnixSockPath = "/tmp/grimlocker.sock"
	WinPipePath  = `\\.\\pipe\\grimlocker`
)
