//go:build !enterprise

package main

import (
	"log"
	"net"
	"net/http"
	"strconv"

	"github.com/grimlocker/grimdb/config/single"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
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

// startSyncListener starts a TCP listener for Local Network Sync.
// Handles incoming sync sessions from trusted peers.
func startSyncListener(provider *single.Provider, blockStore storage.BlockStore, auditLog security.AuditLog) {
	identity := provider.SyncIdentity()
	if identity == nil {
		return
	}
	peerStore := provider.SyncPeerStore()
	syncState := provider.SyncState()
	syncPort := provider.SyncPort()

	ln, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(syncPort)))
	if err != nil {
		log.Printf("[sync:listener] cannot bind port %d: %v (sync listener disabled)", syncPort, err)
		return
	}

	log.Printf("[sync:listener] accepting sync connections on :%d", syncPort)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				log.Printf("[sync:listener] accept error: %v", err)
				return
			}
			go handleSyncConn(conn, identity, peerStore, syncState, blockStore, auditLog)
		}
	}()
}

func handleSyncConn(conn net.Conn, identity *single.DeviceIdentity, peerStore *single.PeerStore, syncState *single.SyncState, blockStore storage.BlockStore, auditLog security.AuditLog) {
	single.HandleIncomingSync(conn, identity, peerStore, syncState, blockStore, auditLog)
}

