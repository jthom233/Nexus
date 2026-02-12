package ipc

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
)

// Handler processes incoming IPC messages from TUI clients.
type Handler func(env *Envelope, reply func(msgType string, payload interface{}) error)

// Server is the GUI-side IPC server that receives commands and sends events.
type Server struct {
	listener net.Listener
	handler  Handler
	clients  []net.Conn
	mu       sync.Mutex
	done     chan struct{}
}

// NewServer creates a new IPC server on the Unix domain socket.
func NewServer(handler Handler) (*Server, error) {
	sockPath := SocketPath()

	// Remove stale socket if it exists
	os.Remove(sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("ipc listen: %w", err)
	}

	return &Server{
		listener: listener,
		handler:  handler,
		done:     make(chan struct{}),
	}, nil
}

// Serve accepts connections and handles messages. Blocks until Close is called.
func (s *Server) Serve() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return nil
			default:
				return fmt.Errorf("ipc accept: %w", err)
			}
		}
		s.mu.Lock()
		s.clients = append(s.clients, conn)
		s.mu.Unlock()
		go s.handleConn(conn)
	}
}

// Broadcast sends an event to all connected TUI clients.
func (s *Server) Broadcast(msgType string, payload interface{}) error {
	data, err := Encode(msgType, payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var active []net.Conn
	for _, c := range s.clients {
		if _, err := c.Write(data); err == nil {
			active = append(active, c)
		}
	}
	s.clients = active
	return nil
}

// Close shuts down the IPC server and cleans up the socket.
func (s *Server) Close() error {
	close(s.done)
	s.mu.Lock()
	for _, c := range s.clients {
		c.Close()
	}
	s.clients = nil
	s.mu.Unlock()
	err := s.listener.Close()
	os.Remove(SocketPath())
	return err
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.removeClient(conn)
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		env, err := Decode(scanner.Bytes())
		if err != nil {
			continue
		}
		reply := func(msgType string, payload interface{}) error {
			data, err := Encode(msgType, payload)
			if err != nil {
				return err
			}
			_, err = conn.Write(data)
			return err
		}
		s.handler(env, reply)
	}
}

func (s *Server) removeClient(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.clients {
		if c == conn {
			s.clients = append(s.clients[:i], s.clients[i+1:]...)
			return
		}
	}
}
