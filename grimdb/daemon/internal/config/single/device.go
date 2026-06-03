//go:build !enterprise

package single

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/blake2b"
)

const (
	deviceKeyFile = "device.key"
	peersFile     = "peers.json"
	pinDigits     = 6
)

// DeviceIdentity holds the Ed25519 keypair for this device.
type DeviceIdentity struct {
	PublicKey  ed25519.PublicKey  `json:"public_key"`
	PrivateKey ed25519.PrivateKey `json:"private_key"`
	DeviceID   string             `json:"device_id"` // BLAKE3(pub)[:16] hex
}

// PeerInfo stores a trusted peer's public key and metadata.
type PeerInfo struct {
	DeviceID   string `json:"device_id"`
	PublicKey  string `json:"public_key"` // base64-encoded ed25519 pub
	PairedAt   int64  `json:"paired_at"`
	LastSeenAt int64  `json:"last_seen_at,omitempty"`
	LastIP     string `json:"last_ip,omitempty"`
}

// PeerStore manages trusted peer identities.
type PeerStore struct {
	mu    sync.RWMutex
	path  string
	peers map[string]PeerInfo
}

// LoadOrCreateIdentity loads an existing device identity or creates a new one.
func LoadOrCreateIdentity(appDir string) (*DeviceIdentity, error) {
	keyPath := filepath.Join(appDir, deviceKeyFile)

	data, err := os.ReadFile(keyPath)
	if err == nil {
		var id DeviceIdentity
		if err := json.Unmarshal(data, &id); err != nil {
			return nil, fmt.Errorf("device: corrupt identity file: %w", err)
		}
		if len(id.PrivateKey) != ed25519.PrivateKeySize || len(id.PublicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("device: identity file has invalid key sizes")
		}
		return &id, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("device: cannot read identity file: %w", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("device: key generation failed: %w", err)
	}

	devID := deviceIDFromPublicKey(pub)
	id := &DeviceIdentity{
		PublicKey:  pub,
		PrivateKey: priv,
		DeviceID:   devID,
	}

	raw, err := json.Marshal(id)
	if err != nil {
		return nil, fmt.Errorf("device: identity marshal failed: %w", err)
	}

	if err := os.WriteFile(keyPath, raw, 0600); err != nil {
		return nil, fmt.Errorf("device: cannot write identity file: %w", err)
	}

	return id, nil
}

// ComputePairingPIN derives the 6-digit pairing PIN from two device public keys + a nonce.
// Both peers compute the same PIN independently, so the user can verify they match.
func ComputePairingPIN(myPub, peerPub ed25519.PublicKey, nonce []byte) string {
	if len(nonce) < 8 {
		nonce = make([]byte, 8)
		_, _ = rand.Read(nonce)
	}

	h := hmac.New(sha256.New, myPub)
	h.Write(peerPub)
	h.Write(nonce)
	mac := h.Sum(nil)

	val := binary.BigEndian.Uint64(mac[:8])
	pin := val % 1000000 // 6-digit PIN range
	return fmt.Sprintf("%06d", pin)
}

// Sign signs a message with the device's Ed25519 private key.
func (d *DeviceIdentity) Sign(msg []byte) []byte {
	return ed25519.Sign(d.PrivateKey, msg)
}

// Verify checks a signature from a peer's public key.
func (d *DeviceIdentity) Verify(msg, sig []byte, peerPub ed25519.PublicKey) bool {
	return ed25519.Verify(peerPub, msg, sig)
}

// deviceIDFromPublicKey derives a short device ID from the public key.
func deviceIDFromPublicKey(pub ed25519.PublicKey) string {
	h := blake2b.Sum256(pub)
	return fmt.Sprintf("%x", h[:16])
}

// ParsePeerPublicKey decodes a base64-encoded Ed25519 public key from a peer.
func ParsePeerPublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("device: invalid peer public key encoding: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("device: invalid peer public key size: got %d, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// NewPeerStore creates a PeerStore that persists to the given app directory.
func NewPeerStore(appDir string) (*PeerStore, error) {
	path := filepath.Join(appDir, peersFile)
	ps := &PeerStore{
		path:  path,
		peers: make(map[string]PeerInfo),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ps, nil
		}
		return nil, fmt.Errorf("device: cannot read peers file: %w", err)
	}

	if err := json.Unmarshal(data, &ps.peers); err != nil {
		return nil, fmt.Errorf("device: corrupt peers file: %w", err)
	}

	return ps, nil
}

// AddPeer adds or updates a trusted peer.
func (ps *PeerStore) AddPeer(info PeerInfo) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.peers[info.DeviceID] = info
	return ps.save()
}

// GetPeer returns a trusted peer by device ID.
func (ps *PeerStore) GetPeer(deviceID string) (PeerInfo, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	info, ok := ps.peers[deviceID]
	return info, ok
}

// RemovePeer removes a trusted peer.
func (ps *PeerStore) RemovePeer(deviceID string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	delete(ps.peers, deviceID)
	return ps.save()
}

// ListPeers returns all trusted peers.
func (ps *PeerStore) ListPeers() []PeerInfo {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	result := make([]PeerInfo, 0, len(ps.peers))
	for _, p := range ps.peers {
		result = append(result, p)
	}
	return result
}

func (ps *PeerStore) save() error {
	data, err := json.MarshalIndent(ps.peers, "", "  ")
	if err != nil {
		return fmt.Errorf("device: peers marshal failed: %w", err)
	}
	if err := os.WriteFile(ps.path, data, 0600); err != nil {
		return fmt.Errorf("device: cannot write peers file: %w", err)
	}
	return nil
}
