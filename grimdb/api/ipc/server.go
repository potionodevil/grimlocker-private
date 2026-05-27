package ipc

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
)

// MessageHandler is called for each authenticated IPC message.
type MessageHandler func(msgType byte, payload []byte, conn net.Conn) error

// Server listens on a Unix domain socket (or named pipe on Windows) and
// authenticates clients via a pre-shared 32-byte cookie.
// After authentication it delegates every message to the registered Handler.
type Server struct {
	cookie  [CookieSize]byte
	handler MessageHandler

	mu      sync.Mutex
	ln      net.Listener
	running bool
}

// NewServer creates an IPC server. cookie must be 32 random bytes.
func NewServer(cookie [CookieSize]byte, handler MessageHandler) *Server {
	return &Server{cookie: cookie, handler: handler}
}

// Start begins accepting connections. Non-blocking — accept loop runs in a goroutine.
func (s *Server) Start() error {
	sockPath := s.SocketPath()
	_ = os.Remove(sockPath)

	var ln net.Listener
	var err error
	if runtime.GOOS == "windows" {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	} else {
		ln, err = net.Listen("unix", sockPath)
	}
	if err != nil {
		return fmt.Errorf("ipc listen %s: %w", sockPath, err)
	}

	s.mu.Lock()
	s.ln = ln
	s.running = true
	s.mu.Unlock()

	log.Printf("[IPC] Listening on %s", sockPath)
	go s.acceptLoop()
	return nil
}

// Stop closes the listener.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	if s.ln != nil {
		if err := s.ln.Close(); err != nil {
			return err
		}
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(s.SocketPath())
	}
	return nil
}

// SocketPath returns the platform-appropriate socket path.
func (s *Server) SocketPath() string {
	if runtime.GOOS == "windows" {
		return WinPipePath
	}
	return UnixSockPath
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			s.mu.Lock()
			running := s.running
			s.mu.Unlock()
			if !running {
				return
			}
			log.Printf("[IPC] Accept error: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	if !s.authenticate(conn) {
		log.Printf("[IPC] Authentication failed")
		return
	}
	log.Printf("[IPC] Client authenticated")

	for {
		msgType, payload, err := ReadMessage(conn)
		if err != nil {
			if err != io.EOF {
				log.Printf("[IPC] Read error: %v", err)
			}
			return
		}
		if err := s.handler(msgType, payload, conn); err != nil {
			log.Printf("[IPC] Handler error: %v", err)
			_ = WriteMessage(conn, MsgError, []byte(err.Error()))
		}
	}
}

func (s *Server) authenticate(conn net.Conn) bool {
	msgType, payload, err := ReadMessage(conn)
	if err != nil || msgType != MsgAck || len(payload) != CookieSize {
		return false
	}
	var recv [CookieSize]byte
	copy(recv[:], payload)
	return recv == s.cookie
}

// ReadMessage reads a length-prefixed message from conn.
func ReadMessage(conn net.Conn) (byte, []byte, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return 0, nil, err
	}
	msgLen := binary.BigEndian.Uint32(lenBuf)
	if msgLen == 0 {
		return 0, nil, fmt.Errorf("zero-length message")
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return 0, nil, err
	}
	return buf[0], buf[1:], nil
}

// WriteMessage sends a length-prefixed message to conn.
func WriteMessage(conn net.Conn, msgType byte, payload []byte) error {
	msgLen := uint32(1 + len(payload))
	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, msgLen)
	if _, err := conn.Write(lenBuf); err != nil {
		return err
	}
	msg := make([]byte, 1+len(payload))
	msg[0] = msgType
	copy(msg[1:], payload)
	_, err := conn.Write(msg)
	return err
}
