package storage

// Block ist die opaque Einheit, die das Storage-Layer liest und schreibt.
// Sie enthält verschlüsselte Bytes, die das Storage-Layer NIEMALS entschlüsselt —
// alles läuft im Crypto-Modul.
type Block struct {
	ID        string   `json:"id"`
	Nonce     []byte   `json:"nonce"`             // 12 bytes
	HMAC      []byte   `json:"hmac"`              // 32 bytes — stored by storage, verified by caller
	Data      []byte   `json:"data"`              // ciphertext
	Category  Category `json:"category,omitempty"` // entry category — written to index for in-memory filtering
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

// BlockMeta enthält nur die nicht-geheimen Metadaten über einen Block.
type BlockMeta struct {
	ID        string   `json:"id"`
	Size      int64    `json:"size"`
	Category  Category `json:"category,omitempty"` // entry category — used for client-side filtering
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}
