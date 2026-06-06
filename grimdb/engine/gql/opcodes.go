// Package gql implementiert das binary-only GQL (GrimQueryLanguage) Frame-Protokoll.
//
// Warum binary-only? Damit es keine Text-Parsing-Angriffe gibt — kein SQL Injection,
// kein JSON-Parser-Exploit. Jedes Field ist längenpräfixiert, was Total Injection Immunity
// bedeutet. Jedes Frame durchläuft eine 2-stufige Validierung (syntaktisch + semantisch/ACL),
// bevor es an den Dispatcher weitergegeben wird.
//
// Frame-Format (8-Byte-Header + Payload):
//
//	Byte 0    : Version       (uint8)
//	Byte 1    : Opcode        (uint8)
//	Bytes 2-3 : Flags         (uint16, big-endian)
//	Bytes 4-7 : PayloadSize   (uint32, big-endian)
//	Bytes 8+  : Payload       (binary-encoded GQLQuery)
//
//	Version 1: Aktuelle Protokollversion.
//	         Alles andere → vom syntaktischen Validator abgewiesen.
package gql

// Version ist die aktuelle GQL Frame-Protocol-Version.
const Version byte = 1

// Opcode sagt dem Empfänger, ob das Frame eine Query, Mutation, Resultat oder ein Error ist.
type Opcode byte

const (
	OpcodeQuery  Opcode = 0x01 // Read-only Query (list, get, search)
	OpcodeMutate Opcode = 0x02 // Write-Operation (create, update, delete)
	OpcodeResult Opcode = 0x03 // Server → Client: Erfolg
	OpcodeError  Opcode = 0x04 // Server → Client: Fehler
)

// Flag ist eine Bitmasken-Konstante im Frame-Header — steuert Compression und Encryption.
type Flag uint16

const (
	FlagNone       Flag = 0x0000
	FlagCompressed Flag = 0x0001 // Payload ist zstd-komprimiert
	FlagEncrypted  Flag = 0x0002 // Payload ist SKE-verschlüsselt
)

// Operation identifiziert die GQL-Operation innerhalb eines Query- oder Mutate-Frames.
type Operation string

// Query-Operationen (read-only, OpcodeQuery) — listen, get und search.
const (
	OpListEntries  Operation = "list_entries"
	OpGetEntry     Operation = "get_entry"
	OpQueryEntries Operation = "query_entries"
)

// Mutate-Operationen (Write, OpcodeMutate) — create, update und delete.
const (
	OpCreateEntry Operation = "create_entry"
	OpUpdateEntry Operation = "update_entry"
	OpDeleteEntry Operation = "delete_entry"
)

// Search-Operationen.
const (
	OpSearchEntries Operation = "search_entries"
)

// File Vault-Operationen — alles rund um Dateien und Ordner im verschlüsselten Vault.
const (
	OpFileListFolder   Operation = "file.list_folder"
	OpFileCreateFolder Operation = "file.create_folder"
	OpFileRenameFolder Operation = "file.rename_folder"
	OpFileDeleteFolder Operation = "file.delete_folder"
	OpFileMove         Operation = "file.move"
	OpFileIngest       Operation = "file.ingest"
	OpFileDownload     Operation = "file.download"
	OpFileUploadStatus Operation = "file.upload_progress"
)

// Workspace-Operationen — Multi-Vault-Management.
const (
	OpWorkspaceList   Operation = "workspace.list"
	OpWorkspaceCreate Operation = "workspace.create"
	OpWorkspaceSwitch Operation = "workspace.switch"
	OpWorkspaceRename Operation = "workspace.rename"
	OpWorkspaceDelete Operation = "workspace.delete"
)

// Sync-Operationen — Peer-to-Peer-Synchronisation.
const (
	OpSyncListPeers Operation = "sync.list_peers"
	OpSyncTrigger   Operation = "sync.trigger"
)

// Audit-Operationen — wer hat wann was gemacht.
const (
	OpAuditList Operation = "audit.list"
)

// Tool-Operationen — SSH-Keys generieren, Recovery-Phrasen ausspucken.
const (
	OpToolSSHGen         Operation = "tool.ssh_gen"
	OpToolRecoveryPhrase Operation = "tool.recovery_phrase"
)

// Vault Auth-Operationen — entsperren, sperren, Status abfragen.
const (
	OpVaultUnlock Operation = "vault.unlock"
	OpVaultLogout Operation = "vault.logout"
	OpVaultStatus Operation = "vault.status"
)

// Health-Operationen — ist der Daemon noch wach?
const (
	OpSystemHealth Operation = "system.health"
)

// isValidOpcode prüft, ob der Opcode ein bekannter Wert ist.
func isValidOpcode(o Opcode) bool {
	switch o {
	case OpcodeQuery, OpcodeMutate, OpcodeResult, OpcodeError:
		return true
	default:
		return false
	}
}

// isValidOperation prüft, ob der Operations-String bekannt ist.
func isValidOperation(op Operation) bool {
	switch op {
	case OpListEntries, OpGetEntry, OpQueryEntries,
		OpCreateEntry, OpUpdateEntry, OpDeleteEntry,
		OpSearchEntries,
		OpFileListFolder, OpFileCreateFolder, OpFileRenameFolder,
		OpFileDeleteFolder, OpFileMove, OpFileIngest, OpFileDownload,
		OpFileUploadStatus,
		OpWorkspaceList, OpWorkspaceCreate, OpWorkspaceSwitch,
		OpWorkspaceRename, OpWorkspaceDelete,
		OpSyncListPeers, OpSyncTrigger,
		OpAuditList,
		OpToolSSHGen, OpToolRecoveryPhrase,
		OpVaultUnlock, OpVaultLogout, OpVaultStatus,
		OpSystemHealth:
		return true
	default:
		return false
	}
}

// isReadOperation sagt, ob eine Operation read-only ist — wichtig für ACL-Checks.
func isReadOperation(op Operation) bool {
	switch op {
	case OpListEntries, OpGetEntry, OpQueryEntries,
		OpSearchEntries,
		OpFileListFolder, OpFileDownload, OpFileUploadStatus,
		OpWorkspaceList,
		OpSyncListPeers,
		OpAuditList,
		OpToolRecoveryPhrase, OpVaultStatus,
		OpSystemHealth:
		return true
	}
	return false
}

// isWriteOperation sagt, ob eine Operation Daten verändert — dann braucht's Credentials.
func isWriteOperation(op Operation) bool {
	switch op {
	case OpCreateEntry, OpUpdateEntry, OpDeleteEntry,
		OpFileCreateFolder, OpFileRenameFolder, OpFileDeleteFolder,
		OpFileMove, OpFileIngest,
		OpWorkspaceCreate, OpWorkspaceSwitch, OpWorkspaceRename,
		OpWorkspaceDelete,
		OpSyncTrigger,
		OpToolSSHGen, OpVaultUnlock, OpVaultLogout:
		return true
	}
	return false
}

// FrameHeaderSize ist die feste Größe des GQL-Frame-Headers — immer 8 Bytes.
const FrameHeaderSize = 8

// MaxPayloadSize ist das absolute Payload-Limit (16 MiB) — schützt vor Memory-Exhaustion.
const MaxPayloadSize = 16 * 1024 * 1024

// MaxNamespaceLen ist die maximale Länge eines Namespace-Strings.
const MaxNamespaceLen = 128

// MaxEntryIDLen ist die maximale Länge einer entry_id.
const MaxEntryIDLen = 64

// MaxCategoryLen maximale Länge einer Category-Bezeichnung.
const MaxCategoryLen = 32

// MaxFieldKeyLen maximale Länge eines einzelnen Field-Keys.
const MaxFieldKeyLen = 64

// MaxFieldValueLen maximale Länge eines einzelnen Field-Werts.
const MaxFieldValueLen = 8192

// MaxFieldsCount maximale Anzahl an Fields pro Entry — verhindert Overload-Attacken.
const MaxFieldsCount = 100

// MaxDataLen maximale Payload-Größe für Bulk-Daten (z.B. FileVault-Chunks).
const MaxDataLen = 10 * 1024 * 1024
