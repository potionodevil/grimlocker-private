package sdk

// Block mirrors storage.Block so plugins can implement storage strategies
// without importing the internal storage package.
type Block struct {
	ID        string `json:"id"`
	Nonce     []byte `json:"nonce"`
	HMAC      []byte `json:"hmac"`
	Data      []byte `json:"data"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// StorageStrategy is the plugin-facing storage strategy interface.
// Implementations are injected into the BlockStore at wire-up time.
type StorageStrategy interface {
	Name() string
	OnWrite(b Block) (Block, error)
	OnRead(b Block) (Block, error)
	OnTrigger(key string) error
}
