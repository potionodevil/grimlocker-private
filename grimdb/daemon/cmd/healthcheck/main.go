// healthcheck is a minimal liveness probe for the Grimlocker enterprise daemon.
// It performs a plain-HTTP GET to the /health endpoint on the internal probe port
// (9090 by default — no mTLS required for liveness checks).
//
// Docker HEALTHCHECK calls this binary; it exits 0 on success, 1 on failure.
// Compiled as a static binary (CGO_ENABLED=0) for use in distroless containers.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("HEALTHCHECK_PORT")
	if port == "" {
		port = "9090"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d\n", resp.StatusCode)
		os.Exit(1)
	}
}
