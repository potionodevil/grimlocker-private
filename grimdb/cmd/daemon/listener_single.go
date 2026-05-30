//go:build !enterprise

package main

import (
	"log"
	"net"
	"net/http"
)

// startTierListener starts the local HTTP listener for the single-user tier.
// Returns the TCP listener and its bound address.
func startTierListener(vault interface{}, ipcMux *http.ServeMux) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", err
	}
	addr := ln.Addr().String()
	log.Printf("[Omega] Single-user IPC listener on %s", addr)
	return ln, addr, nil
}

// tierListenerAddr returns the address advertised on stdout for this tier.
func tierListenerAddr(addr string) string {
	return "ws://" + addr + "/ws"
}
