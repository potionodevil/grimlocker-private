package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
)

// verifyStartupIntegrity computes the SHA-256 of the running daemon binary and
// compares it against an optional expected hash. If expectedHash is empty the
// function only logs the baseline (first-run mode). If expectedHash is provided
// and does not match the computed hash, an error is returned and the daemon
// should refuse to start.
//
// Call this as the very first operation in main() so any binary tampering is
// caught before any keys, secrets, or network sockets are initialised.
//
// On Windows, binaries may be locked by antivirus at startup. If the binary
// cannot be read, the function logs a warning and returns nil (non-fatal) rather
// than producing a false-positive that would prevent legitimate starts.
func verifyStartupIntegrity(expectedHash string) error {
	execPath, err := os.Executable()
	if err != nil {
		log.Printf("[integrity] WARNING: cannot resolve executable path: %v — skipping check", err)
		return nil
	}

	f, err := os.Open(execPath)
	if err != nil {
		// On Windows, antivirus or file locks can transiently block the open.
		// Log but do not fail — the periodic IntegrityMonitor will still catch
		// tampering after the lock is released.
		log.Printf("[integrity] WARNING: cannot open binary for hash: %v — skipping startup check", err)
		return nil
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		log.Printf("[integrity] WARNING: hash computation failed: %v — skipping startup check", err)
		return nil
	}

	computed := hex.EncodeToString(h.Sum(nil))

	if expectedHash == "" {
		// First-run mode: log the baseline so it can be pinned in config.
		log.Printf("[integrity] Baseline SHA-256: %s", computed)
		return nil
	}

	if computed != expectedHash {
		return fmt.Errorf(
			"binary integrity check FAILED: expected=%s got=%s — daemon may have been tampered with",
			expectedHash, computed)
	}

	log.Printf("[integrity] Startup binary integrity OK (%s)", computed[:16]+"...")
	return nil
}
