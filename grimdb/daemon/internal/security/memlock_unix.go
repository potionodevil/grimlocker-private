//go:build !windows

package security

import (
	"fmt"
	"unsafe"

	engine_sec "github.com/grimlocker/grimdb/engine/security"

	"golang.org/x/sys/unix"
)

type unixMemoryGuard struct{}

func NewMemoryGuard() engine_sec.MemoryGuard { return &unixMemoryGuard{} }

func (g *unixMemoryGuard) Lock(b []byte) error {
	if len(b) == 0 { return nil }
	if err := unix.Mlock(b); err != nil { return fmt.Errorf("mlock: %w", err) }
	return nil
}

func (g *unixMemoryGuard) Unlock(b []byte) error {
	if len(b) == 0 { return nil }
	if err := unix.Munlock(b); err != nil { return fmt.Errorf("munlock: %w", err) }
	return nil
}

func (g *unixMemoryGuard) Zeroize(b []byte) { zeroize(b) }

func (g *unixMemoryGuard) CompareConstantTime(a, b []byte) bool {
	if len(a) != len(b) { return false }
	var v byte
	for i := range a { v |= a[i] ^ b[i] }
	return v == 0
}

func (g *unixMemoryGuard) AllocLocked(size int) ([]byte, error) {
	b, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE)
	if err != nil { return nil, fmt.Errorf("mmap: %w", err) }
	if err := unix.Mlock(b); err != nil { _ = unix.Munmap(b); return nil, fmt.Errorf("mlock: %w", err) }
	_ = unsafe.Pointer(&b)
	return b, nil
}
