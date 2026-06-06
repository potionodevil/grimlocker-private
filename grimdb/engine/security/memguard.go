package security

import "unsafe"

// MemoryGuard bietet OS-Level-Memory-Protection-Primitives.
// Implementierungen sind plattformspezifisch (memlock_unix.go / memlock_windows.go).
type MemoryGuard interface {
	// Lock pinnt den Memory-Bereich, damit er nicht auf die Platte ausgelagert wird.
	Lock(b []byte) error

	// Unlock gibt einen vorher gelockten Bereich wieder frei.
	Unlock(b []byte) error

	// Zeroize überschreibt b mit Nullen, so dass der Compiler das nicht wegoptimieren kann.
	Zeroize(b []byte)

	// CompareConstantTime gibt true zurück, wenn a und b gleich sind.
	// Der Vergleich läuft in konstanter Zeit, unabhängig vom Inhalt.
	CompareConstantTime(a, b []byte) bool

	// AllocLocked alloziert einen gezeroiten, memory-gelockten Buffer.
	AllocLocked(size int) ([]byte, error)
}

// zeroize überschreibt b mit Nullen — compiler-resistent.
// Plattformübergreifende Zeroization-Primitive, von SecretGuard genutzt.
func zeroize(b []byte) {
	for i := range b {
		b[i] = 0
	}
	_ = *(*byte)(unsafe.Pointer(&b))
}
