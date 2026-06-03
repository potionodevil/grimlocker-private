//go:build windows

package security

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	engine_sec "github.com/grimlocker/grimdb/engine/security"
)

type windowsMemoryGuard struct{}

func NewMemoryGuard() engine_sec.MemoryGuard { return &windowsMemoryGuard{} }

func (g *windowsMemoryGuard) Lock(b []byte) error {
	if len(b) == 0 { return nil }
	if err := windows.VirtualLock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))); err != nil {
		return fmt.Errorf("VirtualLock: %w", err)
	}
	return nil
}

func (g *windowsMemoryGuard) Unlock(b []byte) error {
	if len(b) == 0 { return nil }
	if err := windows.VirtualUnlock(uintptr(unsafe.Pointer(&b[0])), uintptr(len(b))); err != nil {
		return fmt.Errorf("VirtualUnlock: %w", err)
	}
	return nil
}

func (g *windowsMemoryGuard) Zeroize(b []byte) { zeroize(b) }

func (g *windowsMemoryGuard) CompareConstantTime(a, b []byte) bool {
	if len(a) != len(b) { return false }
	var v byte
	for i := range a { v |= a[i] ^ b[i] }
	return v == 0
}

func (g *windowsMemoryGuard) AllocLocked(size int) ([]byte, error) {
	b := make([]byte, size)
	if err := windows.VirtualLock(uintptr(unsafe.Pointer(&b[0])), uintptr(size)); err != nil {
		return nil, fmt.Errorf("VirtualLock: %w", err)
	}
	return b, nil
}
