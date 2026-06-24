// Package embed ermöglicht die direkte Nutzung von GrimDB als eingebettete
// Datenbank — ohne Daemon, ohne WebSocket, ohne externe Prozesse.
//
// Einsatz: Projekte die GrimDB als lokale verschlüsselte Key-Value-DB nutzen
// möchten, ohne den vollen Grimlocker-Stack zu starten.
package embed

import "github.com/grimlocker/grimdb/engine/storage"

// Category ist der Typ einer Entry-Kategorie im GrimDB Block Store.
type Category = storage.Category

// Vordefinierte Kategorien — kompatibel mit dem Grimlocker-Protokoll.
const (
	CategoryPassword    Category = "PASSWORD"
	CategorySSHKey      Category = "SSH_KEY"
	CategoryCertificate Category = "CERTIFICATE"
	CategoryNote        Category = "NOTE"
	CategoryTOTP        Category = "TOTP"
	CategoryRaw         Category = "RAW" // beliebige Binär- oder JSON-Daten
)

// Block ist ein einzelner entschlüsselter Datensatz aus dem Store.
type Block struct {
	ID        string
	Category  Category
	Data      []byte
	CreatedAt int64 // UnixNano
	UpdatedAt int64 // UnixNano
}

// Config konfiguriert das Embedded-GrimDB.
type Config struct {
	// Dir ist das Verzeichnis, in dem vault.meta, vault_entries.enc,
	// vault_index.enc und vault_wal.enc gespeichert werden.
	Dir string

	// Password ist das Master-Passwort. Es wird per Argon2id zu einem MVK abgeleitet
	// und lebt nur im RAM — nie auf Disk.
	Password string

	// CreateIfMissing: true → neues Vault anlegen falls vault.meta fehlt.
	// Die Recovery-Phrase wird dann nach stdout geschrieben und muss
	// sicher aufbewahrt werden.
	CreateIfMissing bool
}
