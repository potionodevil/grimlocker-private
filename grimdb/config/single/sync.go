//go:build !enterprise

package single

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/chacha20poly1305"

	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
)

const (
	defaultSyncInterval   = 60 * time.Second
	syncBackoffInterval   = 300 * time.Second
	syncBackoffThreshold  = 5
	syncConnectTimeout    = 10 * time.Second
	syncSessionTimeout    = 30 * time.Second
	maxSyncPerPeerPerMin  = 1
)

// EntryVersion tracks the version of an entry for sync purposes.
type EntryVersion struct {
	Version   uint32 `json:"v"` // monotonic version number
	UpdatedAt int64  `json:"t"` // nanosecond timestamp
}

// SyncState persists the last-known version for each entry per peer.
type SyncState struct {
	mu       sync.RWMutex
	path     string
	Versions map[string]EntryVersion `json:"versions"` // entry_id → version
	LastSync map[string]int64        `json:"last_sync"` // peer_device_id → timestamp
}

// LoadSyncState loads or creates a sync state file.
func LoadSyncState(appDir string) (*SyncState, error) {
	path := filepath.Join(appDir, "sync_state.json")
	ss := &SyncState{
		path:     path,
		Versions: make(map[string]EntryVersion),
		LastSync: make(map[string]int64),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ss, nil
		}
		return nil, fmt.Errorf("sync: cannot read state file: %w", err)
	}

	if err := json.Unmarshal(data, &ss); err != nil {
		return nil, fmt.Errorf("sync: corrupt state file: %w", err)
	}
	// Ensure maps are initialized even if JSON had nulls
	if ss.Versions == nil {
		ss.Versions = make(map[string]EntryVersion)
	}
	if ss.LastSync == nil {
		ss.LastSync = make(map[string]int64)
	}
	return ss, nil
}

// UpdateVersion records a new version for an entry.
func (ss *SyncState) UpdateVersion(entryID string, v EntryVersion) {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	current, exists := ss.Versions[entryID]
	if !exists || v.Version > current.Version || (v.Version == current.Version && v.UpdatedAt > current.UpdatedAt) {
		ss.Versions[entryID] = v
	}
}

// GetVersion returns the current version for an entry.
func (ss *SyncState) GetVersion(entryID string) (EntryVersion, bool) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	v, ok := ss.Versions[entryID]
	return v, ok
}

// GetAllVersions returns all current versions.
func (ss *SyncState) GetAllVersions() map[string]EntryVersion {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	result := make(map[string]EntryVersion, len(ss.Versions))
	for k, v := range ss.Versions {
		result[k] = v
	}
	return result
}

// RecordSync marks that a sync occurred with a given peer.
func (ss *SyncState) RecordSync(peerDeviceID string) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.LastSync[peerDeviceID] = time.Now().UnixNano()
}

// Save persists the sync state to disk.
func (ss *SyncState) Save() error {
	ss.mu.RLock()
	defer ss.mu.RUnlock()

	data, err := json.MarshalIndent(ss, "", "  ")
	if err != nil {
		return fmt.Errorf("sync: state marshal failed: %w", err)
	}
	if err := os.WriteFile(ss.path, data, 0600); err != nil {
		return fmt.Errorf("sync: cannot write state file: %w", err)
	}
	return nil
}

// SyncSession represents active sync session state.
type SyncSession struct {
	mu           sync.Mutex
	identity     *DeviceIdentity
	peerStore    *PeerStore
	syncState    *SyncState
	blockStore   storage.BlockStore
	auditLog     security.AuditLog
	sessionKey   []byte
	peerDeviceID string
	nonceSend    uint64
	nonceRecv    uint64
}

// syncMessage types for the sync protocol.
const (
	syncMsgAuthChallenge  = 0x01
	syncMsgAuthResponse   = 0x02
	syncMsgVersionVector  = 0x03
	syncMsgVersionDiff    = 0x04
	syncMsgBlockRequest   = 0x05
	syncMsgBlockData      = 0x06
	syncMsgBlockAck       = 0x07
	syncMsgComplete       = 0x08
	syncMsgError          = 0xFF
)

// syncFrame is the wire format for sync messages.
// 4 bytes: total length (big-endian, excluding these 4 bytes)
// 1 byte:  message type
// 12 bytes: nonce (big-endian uint64 + 4 zero bytes for counter mode)
// N bytes: Poly1305-tagged ciphertext
const (
	syncHeaderLen = 4 + 1 + 12
	syncMaxFrame  = 1 << 20 // 1 MiB max frame
)

// deriveSessionKey derives a session key from two ephemeral contributions.
func deriveSessionKey(contribA, contribB []byte) []byte {
	h, _ := blake2b.New256(nil)
	h.Write(contribA)
	h.Write(contribB)
	h.Write([]byte("grimlocker-sync-session"))
	return h.Sum(nil)
}

// generateNonce creates a random 8-byte nonce for Ed25519 challenges.
func generateNonce() []byte {
	n := make([]byte, 8)
	_, _ = rand.Read(n)
	return n
}

// SyncScheduler manages background polling and sync cycles.
type SyncScheduler struct {
	mu          sync.Mutex
	identity    *DeviceIdentity
	peerStore   *PeerStore
	syncState   *SyncState
	blockStore  storage.BlockStore
	auditLog    security.AuditLog
	sessionCtx  *security.SessionContext
	discovery   *Discovery
	bus         kernel.Dispatcher
	interval    time.Duration
	stopCh      chan struct{}
	missCount   int
}

// NewSyncScheduler creates a SyncScheduler.
func NewSyncScheduler(
	identity *DeviceIdentity,
	peerStore *PeerStore,
	syncState *SyncState,
	blockStore storage.BlockStore,
	auditLog security.AuditLog,
	sessionCtx *security.SessionContext,
	discovery *Discovery,
	bus kernel.Dispatcher,
	interval time.Duration,
) *SyncScheduler {
	if interval <= 0 {
		interval = defaultSyncInterval
	}
	return &SyncScheduler{
		identity:   identity,
		peerStore:  peerStore,
		syncState:  syncState,
		blockStore: blockStore,
		auditLog:   auditLog,
		sessionCtx: sessionCtx,
		discovery:  discovery,
		bus:        bus,
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the background sync loop.
func (s *SyncScheduler) Start() {
	go s.loop()
	log.Printf("[sync:scheduler] started (interval=%s, device=%s)", s.interval, s.identity.DeviceID)
}

// Stop stops the sync scheduler.
func (s *SyncScheduler) Stop() {
	close(s.stopCh)
	log.Printf("[sync:scheduler] stopped")
}

func (s *SyncScheduler) loop() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if !s.sessionCtx.IsUnlocked() {
				continue
			}
			s.trySync()
			ticker.Reset(s.interval)
		}
	}
}

func (s *SyncScheduler) trySync() {
	s.mu.Lock()
	defer s.mu.Unlock()

	peers := s.discovery.GetPeers()

	if len(peers) == 0 {
		s.missCount++
		if s.missCount >= syncBackoffThreshold {
			s.interval = syncBackoffInterval
		}
		return
	}

	s.missCount = 0
	s.interval = defaultSyncInterval

	for _, peer := range peers {
		if peer.DeviceID == s.identity.DeviceID {
			continue
		}

		trustedPeer, ok := s.peerStore.GetPeer(peer.DeviceID)
		if !ok {
			continue
		}

		if err := s.syncWithPeer(peer, trustedPeer); err != nil {
			log.Printf("[sync:scheduler] sync with %s failed: %v", peer.DeviceID, err)
		}
	}
}

func (s *SyncScheduler) syncWithPeer(peer DiscoveredPeer, trusted PeerInfo) error {
	log.Printf("[sync] beginning sync with %s (%s)", peer.DeviceID, peer.Host)

	if s.bus != nil {
		payload, _ := json.Marshal(map[string]string{
			"peer_device_id": peer.DeviceID,
			"peer_host":      peer.Host,
		})
		_ = s.bus.Dispatch(kernel.NewEvent("sync", kernel.EvSyncBegin, payload))
	}

	peerPub, err := ParsePeerPublicKey(trusted.PublicKey)
	if err != nil {
		return fmt.Errorf("sync: invalid peer public key: %w", err)
	}

	conn, err := net.DialTimeout("tcp", peer.Host, syncConnectTimeout)
	if err != nil {
		s.auditLog.Append(security.SecurityEvent{
			Level:     security.LevelWarn,
			Module:    "sync",
			Message:   fmt.Sprintf("sync connect failed: %s", peer.DeviceID),
			SubjectID: peer.DeviceID,
		})
		return fmt.Errorf("sync: connect to %s: %w", peer.Host, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(syncSessionTimeout))

	sess := &SyncSession{
		identity:     s.identity,
		peerStore:    s.peerStore,
		syncState:    s.syncState,
		blockStore:   s.blockStore,
		auditLog:     s.auditLog,
		peerDeviceID: peer.DeviceID,
	}

	s.auditLog.Append(security.SecurityEvent{
		Level:     security.LevelInfo,
		Module:    "sync",
		Message:   fmt.Sprintf("sync session started: %s", peer.DeviceID),
		SubjectID: peer.DeviceID,
	})

	if err := sess.handshake(conn, peerPub); err != nil {
		s.auditLog.Append(security.SecurityEvent{
			Level:     security.LevelWarn,
			Module:    "sync",
			Message:   fmt.Sprintf("sync auth failed: %s: %v", peer.DeviceID, err),
			SubjectID: peer.DeviceID,
		})
		return fmt.Errorf("sync: handshake failed: %w", err)
	}

	if err := sess.exchangeEntries(conn, peer.Version); err != nil {
		s.auditLog.Append(security.SecurityEvent{
			Level:     security.LevelWarn,
			Module:    "sync",
			Message:   fmt.Sprintf("sync exchange failed: %s: %v", peer.DeviceID, err),
			SubjectID: peer.DeviceID,
		})
		return fmt.Errorf("sync: exchange failed: %w", err)
	}

	s.syncState.RecordSync(peer.DeviceID)

	s.auditLog.Append(security.SecurityEvent{
		Level:     security.LevelInfo,
		Module:    "sync",
		Message:   fmt.Sprintf("sync session completed: %s", peer.DeviceID),
		SubjectID: peer.DeviceID,
	})

	if s.bus != nil {
		payload, _ := json.Marshal(map[string]string{
			"peer_device_id": peer.DeviceID,
			"status":         "completed",
		})
		_ = s.bus.Dispatch(kernel.NewEvent("sync", kernel.EvSyncComplete, payload))
	}

	return nil
}

func (sess *SyncSession) handshake(conn net.Conn, peerPub ed25519.PublicKey) error {
	// 1. Generate ephemeral key contribution
	myContrib := make([]byte, 32)
	if _, err := rand.Read(myContrib); err != nil {
		return fmt.Errorf("handshake: contrib generation failed: %w", err)
	}

	// 2. Generate challenge nonce
	challengeNonce := generateNonce()

	// 3. Send: challenge_nonce || my_contrib || sig(challenge_nonce || my_contrib)
	challengeMsg := make([]byte, 8+32+ed25519.SignatureSize)
	copy(challengeMsg[:8], challengeNonce)
	copy(challengeMsg[8:40], myContrib)
	sig := sess.identity.Sign(challengeMsg[:40])
	copy(challengeMsg[40:], sig)

	if _, err := writeFrame(conn, syncMsgAuthChallenge, challengeMsg); err != nil {
		return fmt.Errorf("handshake: write challenge: %w", err)
	}

	// 4. Read peer's challenge and their contribution
	peerChallenge, err := readFrame(conn, syncMsgAuthChallenge)
	if err != nil {
		return fmt.Errorf("handshake: read peer challenge: %w", err)
	}
	if len(peerChallenge) < 40+ed25519.SignatureSize {
		return fmt.Errorf("handshake: peer challenge too short: %d", len(peerChallenge))
	}

	peerNonce := peerChallenge[:8]
	peerContrib := peerChallenge[8:40]
	peerSig := peerChallenge[40 : 40+ed25519.SignatureSize]

	if !sess.identity.Verify(peerChallenge[:40], peerSig, peerPub) {
		return fmt.Errorf("handshake: peer signature verification failed")
	}

	// 5. Send response: peer_nonce || sig(peer_nonce) — proves we hold our private key
	responseSig := sess.identity.Sign(peerNonce)
	responseMsg := make([]byte, 8+ed25519.SignatureSize)
	copy(responseMsg[:8], peerNonce)
	copy(responseMsg[8:], responseSig)

	if _, err := writeFrame(conn, syncMsgAuthResponse, responseMsg); err != nil {
		return fmt.Errorf("handshake: write response: %w", err)
	}

	// 6. Read peer's response to our challenge
	peerResponse, err := readFrame(conn, syncMsgAuthResponse)
	if err != nil {
		return fmt.Errorf("handshake: read peer response: %w", err)
	}
	if len(peerResponse) < 8+ed25519.SignatureSize {
		return fmt.Errorf("handshake: peer response too short: %d", len(peerResponse))
	}

	peerRespNonce := peerResponse[:8]
	peerRespSig := peerResponse[8 : 8+ed25519.SignatureSize]

	if !sess.identity.Verify(peerRespNonce, peerRespSig, peerPub) {
		return fmt.Errorf("handshake: peer response verification failed")
	}

	// 7. Verify the peer echoed our challenge nonce
	if string(peerRespNonce) != string(challengeNonce) {
		return fmt.Errorf("handshake: challenge nonce mismatch")
	}

	// 8. Derive session key
	sess.sessionKey = deriveSessionKey(myContrib, peerContrib)

	log.Printf("[sync:handshake] authenticated peer %s", sess.peerDeviceID)
	return nil
}

func (sess *SyncSession) exchangeEntries(conn net.Conn, peerRemoteVersions map[string]EntryVersion) error {
	// 1. Send our version vector
	localVersions := sess.syncState.GetAllVersions()
	vvPayload, _ := json.Marshal(localVersions)

	encryptedVV, err := sess.encryptFrame(vvPayload)
	if err != nil {
		return fmt.Errorf("exchange: encrypt version vector: %w", err)
	}
	if _, err := writeFrame(conn, syncMsgVersionVector, encryptedVV); err != nil {
		return fmt.Errorf("exchange: write version vector: %w", err)
	}

	// 2. Read peer's encrypted version vector
	peerEncryptedVV, err := readFrame(conn, syncMsgVersionVector)
	if err != nil {
		return fmt.Errorf("exchange: read peer version vector: %w", err)
	}

	peerVVPayload, err := sess.decryptFrame(peerEncryptedVV)
	if err != nil {
		return fmt.Errorf("exchange: decrypt peer version vector: %w", err)
	}

	var peerVersions map[string]EntryVersion
	if err := json.Unmarshal(peerVVPayload, &peerVersions); err != nil {
		return fmt.Errorf("exchange: unmarshal peer version vector: %w", err)
	}

	// 3. Determine which entries are newer on peer
	var newerIDs []string
	for entryID, peerVer := range peerVersions {
		localVer, exists := sess.syncState.GetVersion(entryID)
		if !exists || peerVer.Version > localVer.Version || (peerVer.Version == localVer.Version && peerVer.UpdatedAt > localVer.UpdatedAt) {
			newerIDs = append(newerIDs, entryID)
		}
	}

	if len(newerIDs) == 0 {
		log.Printf("[sync:exchange] no new entries from %s", sess.peerDeviceID)
		if _, err := writeFrame(conn, syncMsgComplete, []byte("up-to-date")); err != nil {
			return fmt.Errorf("exchange: write complete: %w", err)
		}
		return nil
	}

	// 4. Request blocks for newer entries
	reqPayload, _ := json.Marshal(map[string][]string{"entry_ids": newerIDs})
	encryptedReq, err := sess.encryptFrame(reqPayload)
	if err != nil {
		return fmt.Errorf("exchange: encrypt block request: %w", err)
	}
	if _, err := writeFrame(conn, syncMsgBlockRequest, encryptedReq); err != nil {
		return fmt.Errorf("exchange: write block request: %w", err)
	}

	// 5. Read blocks from peer
	for i := 0; i < len(newerIDs); i++ {
		blockData, err := readFrame(conn, syncMsgBlockData)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("exchange: read block %d: %w", i, err)
		}

		if len(blockData) < 17 {
			return fmt.Errorf("exchange: block %d too short: %d", i, len(blockData))
		}

		// Decrypt the block
		plainBlock, err := sess.decryptFrame(blockData)
		if err != nil {
			log.Printf("[sync:exchange] block %d decrypt failed: %v", i, err)
			continue
		}

		var block storage.Block
		if err := json.Unmarshal(plainBlock, &block); err != nil {
			log.Printf("[sync:exchange] block %d unmarshal failed: %v", i, err)
			continue
		}

		if block.ID == "" {
			continue
		}

		// Write to local block store
		if err := sess.blockStore.WriteBlock(block); err != nil {
			log.Printf("[sync:exchange] write block %s: %v", block.ID, err)
			continue
		}

		// Update sync state
		if version, ok := peerVersions[block.ID]; ok {
			sess.syncState.UpdateVersion(block.ID, version)
		}

		sess.auditLog.Append(security.SecurityEvent{
			Level:     security.LevelInfo,
			Module:    "sync",
			Message:   fmt.Sprintf("entry synced: %s from %s", block.ID, sess.peerDeviceID),
			SubjectID: block.ID,
		})

		// Acknowledge receipt
		ack, _ := json.Marshal(map[string]string{"id": block.ID, "status": "ok"})
		encAck, _ := sess.encryptFrame(ack)
		writeFrame(conn, syncMsgBlockAck, encAck) //nolint:errcheck
	}

	// 6. Signal completion
	if _, err := writeFrame(conn, syncMsgComplete, []byte("done")); err != nil {
		return fmt.Errorf("exchange: write complete: %w", err)
	}

	if err := sess.syncState.Save(); err != nil {
		log.Printf("[sync:exchange] save state: %v", err)
	}

	log.Printf("[sync:exchange] synced %d entries from %s", len(newerIDs), sess.peerDeviceID)
	return nil
}

func writeFrame(conn net.Conn, msgType byte, payload []byte) (int, error) {
	frameLen := 1 + len(payload)
	buf := make([]byte, 4+frameLen)
	binary.BigEndian.PutUint32(buf[:4], uint32(frameLen))
	buf[4] = msgType
	copy(buf[5:], payload)
	return conn.Write(buf)
}

func readFrame(conn net.Conn, expectedType byte) ([]byte, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, fmt.Errorf("read len: %w", err)
	}
	frameLen := binary.BigEndian.Uint32(lenBuf)
	if frameLen > syncMaxFrame {
		return nil, fmt.Errorf("frame too large: %d", frameLen)
	}
	if frameLen < 1 {
		return nil, fmt.Errorf("frame too short: %d", frameLen)
	}

	frame := make([]byte, frameLen)
	if _, err := io.ReadFull(conn, frame); err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	msgType := frame[0]
	if msgType == syncMsgError {
		return nil, fmt.Errorf("peer error: %s", string(frame[1:]))
	}
	if msgType != expectedType {
		return nil, fmt.Errorf("unexpected message type: got 0x%02x, want 0x%02x", msgType, expectedType)
	}

	return frame[1:], nil
}

// encryptFrame encrypts payload with the session key using ChaCha20-Poly1305.
// Wire format: 12-byte nonce || ciphertext+tag
func (sess *SyncSession) encryptFrame(payload []byte) ([]byte, error) {
	if sess.sessionKey == nil {
		return nil, fmt.Errorf("sync: no session key")
	}

	sess.mu.Lock()
	nonce := sess.nonceSend
	sess.nonceSend++
	sess.mu.Unlock()

	// Build 12-byte nonce: 8 bytes counter + 4 zero bytes
	n := make([]byte, 12)
	binary.BigEndian.PutUint64(n[:8], nonce)

	// Use ChaCha20-Poly1305 via x/crypto
	ct, err := encryptWithSessionKey(sess.sessionKey, n, payload)
	if err != nil {
		return nil, err
	}

	result := make([]byte, 12+len(ct))
	copy(result[:12], n)
	copy(result[12:], ct)
	return result, nil
}

// decryptFrame decrypts a frame encrypted with encryptFrame.
func (sess *SyncSession) decryptFrame(data []byte) ([]byte, error) {
	if sess.sessionKey == nil {
		return nil, fmt.Errorf("sync: no session key")
	}
	if len(data) < 12+16 {
		return nil, fmt.Errorf("sync: frame too short for AEAD: %d", len(data))
	}

	nonce := data[:12]
	ct := data[12:]

	return decryptWithSessionKey(sess.sessionKey, nonce, ct)
}

func encryptWithSessionKey(key, nonce, plaintext []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("sync: invalid key size: %d", len(key))
	}
	if len(nonce) != 12 {
		return nil, fmt.Errorf("sync: invalid nonce size: %d", len(nonce))
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("sync: chacha20poly1305 init: %w", err)
	}
	return aead.Seal(nil, nonce, plaintext, nil), nil
}

func decryptWithSessionKey(key, nonce, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < chacha20poly1305.Overhead {
		return nil, fmt.Errorf("sync: ciphertext too short: %d", len(ciphertext))
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("sync: chacha20poly1305 init: %w", err)
	}
	return aead.Open(nil, nonce, ciphertext, nil)
}

// HandleIncomingSync handles a sync connection initiated by a remote peer.
// This is the server-side handler called by the sync TCP listener.
func HandleIncomingSync(conn net.Conn, identity *DeviceIdentity, peerStore *PeerStore, syncState *SyncState, blockStore storage.BlockStore, auditLog security.AuditLog) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(syncSessionTimeout))

	sess := &SyncSession{
		identity:   identity,
		peerStore:  peerStore,
		syncState:  syncState,
		blockStore: blockStore,
		auditLog:   auditLog,
	}

	// Determine which peer is connecting by performing the handshake.
	// We need to know the peer's public key to verify. Since we don't know
	// which peer is connecting yet, the peer sends its device ID first.
	deviceIDBuf := make([]byte, 64)
	n, err := conn.Read(deviceIDBuf)
	if err != nil || n < 16 {
		log.Printf("[sync:incoming] failed to read peer device ID: %v", err)
		return
	}
	peerDeviceID := string(deviceIDBuf[:n])
	sess.peerDeviceID = peerDeviceID

	trustedPeer, ok := peerStore.GetPeer(peerDeviceID)
	if !ok {
		log.Printf("[sync:incoming] unknown peer: %s", peerDeviceID)
		auditLog.Append(security.SecurityEvent{
			Level:     security.LevelWarn,
			Module:    "sync",
			Message:   fmt.Sprintf("rejected unknown peer: %s", peerDeviceID),
			SubjectID: peerDeviceID,
		})
		return
	}

	peerPub, err := ParsePeerPublicKey(trustedPeer.PublicKey)
	if err != nil {
		log.Printf("[sync:incoming] invalid peer key: %v", err)
		return
	}

	auditLog.Append(security.SecurityEvent{
		Level:     security.LevelInfo,
		Module:    "sync",
		Message:   fmt.Sprintf("incoming sync from %s", peerDeviceID),
		SubjectID: peerDeviceID,
	})

	if err := sess.handshake(conn, peerPub); err != nil {
		auditLog.Append(security.SecurityEvent{
			Level:     security.LevelWarn,
			Module:    "sync",
			Message:   fmt.Sprintf("sync handshake failed for %s: %v", peerDeviceID, err),
			SubjectID: peerDeviceID,
		})
		return
	}

	if err := sess.serveEntries(conn); err != nil {
		auditLog.Append(security.SecurityEvent{
			Level:     security.LevelWarn,
			Module:    "sync",
			Message:   fmt.Sprintf("serve entries to %s failed: %v", peerDeviceID, err),
			SubjectID: peerDeviceID,
		})
		return
	}

	syncState.RecordSync(peerDeviceID)
	if err := syncState.Save(); err != nil {
		log.Printf("[sync:incoming] save state: %v", err)
	}

	auditLog.Append(security.SecurityEvent{
		Level:     security.LevelInfo,
		Module:    "sync",
		Message:   fmt.Sprintf("incoming sync completed: %s", peerDeviceID),
		SubjectID: peerDeviceID,
	})
}

// serveEntries handles the server side of the sync exchange.
// Reads peer's version vector, computes diff, and serves requested blocks.
func (sess *SyncSession) serveEntries(conn net.Conn) error {
	// 1. Read peer's encrypted version vector
	peerEncryptedVV, err := readFrame(conn, syncMsgVersionVector)
	if err != nil {
		return fmt.Errorf("serve: read version vector: %w", err)
	}

	peerVVPayload, err := sess.decryptFrame(peerEncryptedVV)
	if err != nil {
		return fmt.Errorf("serve: decrypt version vector: %w", err)
	}

	var peerVersions map[string]EntryVersion
	if err := json.Unmarshal(peerVVPayload, &peerVersions); err != nil {
		return fmt.Errorf("serve: unmarshal peer version vector: %w", err)
	}

	// 2. Send our version vector
	localVersions := sess.syncState.GetAllVersions()
	vvPayload, _ := json.Marshal(localVersions)

	encryptedVV, err := sess.encryptFrame(vvPayload)
	if err != nil {
		return fmt.Errorf("serve: encrypt version vector: %w", err)
	}
	if _, err := writeFrame(conn, syncMsgVersionVector, encryptedVV); err != nil {
		return fmt.Errorf("serve: write version vector: %w", err)
	}

	// 3. Read block request
	reqEncrypted, err := readFrame(conn, syncMsgBlockRequest)
	if err != nil {
		if err == io.EOF || isErrorFrame(err) {
			return nil // peer has nothing to request
		}
		return fmt.Errorf("serve: read block request: %w", err)
	}

	reqPayload, err := sess.decryptFrame(reqEncrypted)
	if err != nil {
		return fmt.Errorf("serve: decrypt block request: %w", err)
	}

	var req struct {
		EntryIDs []string `json:"entry_ids"`
	}
	if err := json.Unmarshal(reqPayload, &req); err != nil {
		return fmt.Errorf("serve: unmarshal block request: %w", err)
	}

	// 4. Serve each requested block
	for _, entryID := range req.EntryIDs {
		block, err := sess.blockStore.ReadBlock(entryID)
		if err != nil {
			log.Printf("[sync:serve] read block %s: %v", entryID, err)
			continue
		}

		blockPayload, _ := json.Marshal(block)
		encBlock, err := sess.encryptFrame(blockPayload)
		if err != nil {
			log.Printf("[sync:serve] encrypt block %s: %v", entryID, err)
			continue
		}

		if _, err := writeFrame(conn, syncMsgBlockData, encBlock); err != nil {
			return fmt.Errorf("serve: write block %s: %w", entryID, err)
		}

		// Read ack
		_, err = readFrame(conn, syncMsgBlockAck)
		if err != nil {
			log.Printf("[sync:serve] ack for %s: %v", entryID, err)
		}
	}

	// 5. Wait for completion signal
	_, _ = readFrame(conn, syncMsgComplete)

	log.Printf("[sync:serve] served %d blocks to %s", len(req.EntryIDs), sess.peerDeviceID)
	return nil
}

func isErrorFrame(err error) bool {
	return err != nil && err.Error() != "read len: EOF"
}

// TriggerNow fires an immediate sync cycle outside the regular schedule.
// Non-blocking: runs in a background goroutine.
func (s *SyncScheduler) TriggerNow() {
	go func() {
		if s.sessionCtx == nil || !s.sessionCtx.IsUnlocked() {
			return
		}
		s.trySync()
	}()
}

// LastSyncAt returns the most recent sync timestamp across all known peers.
// Returns the zero Time if no sync has occurred yet.
func (s *SyncScheduler) LastSyncAt() time.Time {
	if s.syncState == nil {
		return time.Time{}
	}
	s.syncState.mu.RLock()
	defer s.syncState.mu.RUnlock()

	var latest int64
	for _, ts := range s.syncState.LastSync {
		if ts > latest {
			latest = ts
		}
	}
	if latest == 0 {
		return time.Time{}
	}
	return time.Unix(0, latest)
}
