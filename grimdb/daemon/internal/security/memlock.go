package security

import "unsafe"

// zeroize overwrites b with zeros in a way the compiler cannot elide.
// Used by platform-specific MemoryGuard implementations.
func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
	_ = *(*byte)(unsafe.Pointer(&b))
}
