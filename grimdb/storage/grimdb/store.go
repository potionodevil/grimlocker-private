package grimdb

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	HeaderSize       = 26
	MaxFailedAttempts = 3
	LockdownMinutes   = 200
	OverrideAttempts  = 4
)

// Header is the 26-byte binary header of the .gdb file.
// Layout (big-endian):
//   [0]     failed_attempts       uint8
//   [1:9]   lockdown_timestamp    int64
//   [9]     override_attempts_left uint8
//   [10:18] monotonic_boot_ticks  uint64
//   [18:26] wallclock_last_seen   int64
type Header struct {
	FailedAttempts       uint8
	LockdownTimestamp    int64
	OverrideAttemptsLeft uint8
	MonotonicBootTicks   uint64
	WallclockLastSeen    int64
}

// GrimDB manages the .gdb file's header and ciphertext payload.
// It knows nothing about encryption — it treats the body as opaque bytes.
type GrimDB struct {
	mu       sync.RWMutex
	filePath string
	appDir   string
	header   Header
}

// NewGrimDB opens or creates the .gdb file at filePath.
func NewGrimDB(filePath string) (*GrimDB, error) {
	g := &GrimDB{
		filePath: filePath,
		appDir:   filepath.Dir(filePath),
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := g.createEmpty(); err != nil {
			return nil, fmt.Errorf("create empty gdb: %w", err)
		}
	}

	if err := g.loadHeader(); err != nil {
		return nil, fmt.Errorf("load header: %w", err)
	}
	return g, nil
}

func (g *GrimDB) createEmpty() error {
	dir := filepath.Dir(g.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(g.filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(make([]byte, HeaderSize))
	return err
}

func (g *GrimDB) loadHeader() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	f, err := os.OpenFile(g.filePath, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, HeaderSize)
	n, err := f.Read(buf)
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if n < HeaderSize {
		return fmt.Errorf("header too short: got %d want %d", n, HeaderSize)
	}

	g.header.FailedAttempts = buf[0]
	g.header.LockdownTimestamp = int64(binary.BigEndian.Uint64(buf[1:9]))
	g.header.OverrideAttemptsLeft = buf[9]
	g.header.MonotonicBootTicks = binary.BigEndian.Uint64(buf[10:18])
	g.header.WallclockLastSeen = int64(binary.BigEndian.Uint64(buf[18:26]))
	return nil
}

func (g *GrimDB) GetHeader() Header {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.header
}

func (g *GrimDB) UpdateHeader(h Header) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.header = h
	return g.atomicWriteHeader(headerToBytes(h))
}

func (g *GrimDB) RefreshTimeFields() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.header.WallclockLastSeen = time.Now().Unix()
	return g.atomicWriteHeader(headerToBytes(g.header))
}

func (g *GrimDB) GetCiphertext() ([]byte, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	f, err := os.OpenFile(g.filePath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size() <= int64(HeaderSize) {
		return []byte{}, nil
	}

	payloadSize := fi.Size() - int64(HeaderSize)
	buf := make([]byte, payloadSize)
	if _, err := f.ReadAt(buf, int64(HeaderSize)); err != nil {
		return nil, fmt.Errorf("read ciphertext: %w", err)
	}
	return buf, nil
}

func (g *GrimDB) UpdateCiphertext(ciphertext []byte) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	tmpPath := g.filePath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(headerToBytes(g.header)); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := f.Write(ciphertext); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, g.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (g *GrimDB) GetFilePath() string { return g.filePath }
func (g *GrimDB) GetAppDir() string   { return g.appDir }

func (g *GrimDB) atomicWriteHeader(newHeader []byte) error {
	tmpPath := g.filePath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write(newHeader); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, g.filePath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func headerToBytes(h Header) []byte {
	buf := make([]byte, HeaderSize)
	buf[0] = h.FailedAttempts
	binary.BigEndian.PutUint64(buf[1:9], uint64(h.LockdownTimestamp))
	buf[9] = h.OverrideAttemptsLeft
	binary.BigEndian.PutUint64(buf[10:18], h.MonotonicBootTicks)
	binary.BigEndian.PutUint64(buf[18:26], uint64(h.WallclockLastSeen))
	return buf
}
