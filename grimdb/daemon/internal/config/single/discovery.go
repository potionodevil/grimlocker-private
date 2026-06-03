//go:build !enterprise

package single

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	mdnsAddr     = "224.0.0.251:5353"
	syncPort     = 9735
	serviceType  = "_grimlocker._tcp"
	discoveryTTL = 1
)

// DiscoveredPeer represents a peer found via mDNS.
type DiscoveredPeer struct {
	DeviceID string `json:"device_id"`
	Host     string `json:"host"` // IP:port
	Port     int    `json:"port"`
	Version  map[string]EntryVersion `json:"version,omitempty"`
	SeenAt   int64  `json:"seen_at"`
}

// Discovery manages mDNS-based peer discovery.
type Discovery struct {
	mu         sync.RWMutex
	deviceID   string
	port       int
	conn       *net.UDPConn
	peers      map[string]DiscoveredPeer
	stopCh     chan struct{}
	versionFn  func() map[string]EntryVersion
}

// versionPayload is the version vector broadcast via mDNS TXT records.
type versionPayload struct {
	DeviceID string                   `json:"d"`
	Version  map[string]EntryVersion  `json:"v"`
	Port     int                      `json:"p"`
}

// NewDiscovery creates a Discovery instance for the given device.
func NewDiscovery(deviceID string, port int, versionFn func() map[string]EntryVersion) *Discovery {
	return &Discovery{
		deviceID:  deviceID,
		port:      port,
		peers:     make(map[string]DiscoveredPeer),
		stopCh:    make(chan struct{}),
		versionFn: versionFn,
	}
}

// Start begins mDNS advertisement and discovery.
func (d *Discovery) Start() error {
	addr, err := net.ResolveUDPAddr("udp", mdnsAddr)
	if err != nil {
		return fmt.Errorf("discovery: resolve mDNS addr: %w", err)
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("discovery: listen multicast: %w", err)
	}
	conn.SetReadBuffer(4096)
	d.conn = conn

	go d.listen()
	go d.advertise()

	log.Printf("[sync:discovery] mDNS started on %s (device=%s)", mdnsAddr, d.deviceID)
	return nil
}

// Stop shuts down discovery.
func (d *Discovery) Stop() {
	close(d.stopCh)
	if d.conn != nil {
		d.conn.Close()
	}
}

// GetPeers returns all currently discovered peers.
func (d *Discovery) GetPeers() []DiscoveredPeer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]DiscoveredPeer, 0, len(d.peers))
	for _, p := range d.peers {
		result = append(result, p)
	}
	return result
}

// GetPeer returns a specific discovered peer by device ID.
func (d *Discovery) GetPeer(deviceID string) (DiscoveredPeer, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	p, ok := d.peers[deviceID]
	return p, ok
}

func (d *Discovery) listen() {
	buf := make([]byte, 1500)
	for {
		select {
		case <-d.stopCh:
			return
		default:
		}

		d.conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, src, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if strings.Contains(err.Error(), "closed") {
				return
			}
			log.Printf("[sync:discovery] read error: %v", err)
			continue
		}

		d.handleAdvertisement(buf[:n], src.IP.String())
	}
}

func (d *Discovery) handleAdvertisement(data []byte, srcIP string) {
	// Parse TXT record: key=value pairs separated by null bytes or newlines
	lines := strings.Split(string(data), "\n")
	var vp versionPayload
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "grimlocker=") {
			raw := strings.TrimPrefix(line, "grimlocker=")
			if err := json.Unmarshal([]byte(raw), &vp); err != nil {
				log.Printf("[sync:discovery] invalid TXT from %s: %v", srcIP, err)
				return
			}
		}
	}

	if vp.DeviceID == "" || vp.DeviceID == d.deviceID {
		return
	}

	port := vp.Port
	if port == 0 {
		port = syncPort
	}

	d.mu.Lock()
	d.peers[vp.DeviceID] = DiscoveredPeer{
		DeviceID: vp.DeviceID,
		Host:     fmt.Sprintf("%s:%d", srcIP, port),
		Port:     port,
		Version:  vp.Version,
		SeenAt:   time.Now().UnixNano(),
	}
	d.mu.Unlock()
}

func (d *Discovery) advertise() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	d.sendAdvertisement() // initial broadcast

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.sendAdvertisement()
		}
	}
}

func (d *Discovery) sendAdvertisement() {
	var versions map[string]EntryVersion
	if d.versionFn != nil {
		versions = d.versionFn()
	}
	if versions == nil {
		versions = make(map[string]EntryVersion)
	}

	vp := versionPayload{
		DeviceID: d.deviceID,
		Version:  versions,
		Port:     d.port,
	}

	payload, err := json.Marshal(vp)
	if err != nil {
		log.Printf("[sync:discovery] version marshal error: %v", err)
		return
	}

	msg := fmt.Sprintf("grimlocker=%s\n", string(payload))

	addr, err := net.ResolveUDPAddr("udp", mdnsAddr)
	if err != nil {
		return
	}

	if d.conn != nil {
		_, err := d.conn.WriteToUDP([]byte(msg), addr)
		if err != nil {
			log.Printf("[sync:discovery] advertise error: %v", err)
		}
	}
}
