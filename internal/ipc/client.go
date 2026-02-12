package ipc

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

// Client is the TUI-side IPC client that sends commands and receives events.
type Client struct {
	conn    net.Conn
	scanner *bufio.Scanner
	mu      sync.Mutex
}

// Connect connects to the GUI process via Unix domain socket.
func Connect(timeout time.Duration) (*Client, error) {
	sockPath := SocketPath()
	deadline := time.Now().Add(timeout)

	var conn net.Conn
	var err error
	for time.Now().Before(deadline) {
		conn, err = net.DialTimeout("unix", sockPath, 2*time.Second)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("ipc connect: %w", err)
	}

	return &Client{
		conn:    conn,
		scanner: bufio.NewScanner(conn),
	}, nil
}

// Send sends a command to the GUI process.
func (c *Client) Send(msgType string, payload interface{}) error {
	data, err := Encode(msgType, payload)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = c.conn.Write(data)
	return err
}

// Recv blocks until the next event from the GUI process.
func (c *Client) Recv() (*Envelope, error) {
	if !c.scanner.Scan() {
		if err := c.scanner.Err(); err != nil {
			return nil, fmt.Errorf("ipc recv: %w", err)
		}
		return nil, fmt.Errorf("ipc: connection closed")
	}
	return Decode(c.scanner.Bytes())
}

// Close closes the IPC connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Probe checks if the GUI process is listening on the socket.
func Probe() bool {
	conn, err := net.DialTimeout("unix", SocketPath(), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
