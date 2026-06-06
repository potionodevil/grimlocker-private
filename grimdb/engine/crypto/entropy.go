package crypto

import (
	"crypto/rand"
	"fmt"
	"os"
)

// GenerateEntropyFileWithProgress generiert eine zufällige Entropy-Datei am gegebenen Pfad
// und ruft progressFn periodisch mit dem Fortschritt (0.0–1.0) und einer Message auf.
// Schreibt in Chunks; die Gesamtgröße ist 2MB (2097152 Bytes).
func (p *provider) GenerateEntropyFileWithProgress(path string, progressFn func(pct float64, msg string)) error {
	const totalSize = 2 * 1024 * 1024
	const chunkSize = 64 * 1024

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("open entropy file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, chunkSize)
	written := 0

	for written < totalSize {
		if _, err := rand.Read(buf); err != nil {
			return fmt.Errorf("read entropy: %w", err)
		}

		toWrite := totalSize - written
		if toWrite > len(buf) {
			toWrite = len(buf)
		}

		if _, err := f.Write(buf[:toWrite]); err != nil {
			return fmt.Errorf("write entropy: %w", err)
		}

		written += toWrite

		pct := float64(written) / float64(totalSize)
		progressFn(pct, fmt.Sprintf("Generated %d / %d bytes", written, totalSize))
	}

	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync entropy file: %w", err)
	}

	return nil
}
