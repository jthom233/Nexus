package session

import (
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// FileEntry represents a single file or directory in an SFTP listing.
type FileEntry struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool
}

// SFTPSession wraps an SFTP client and its underlying SSH connection.
type SFTPSession struct {
	client *sftp.Client
	conn   *ssh.Client
}

// NewSFTPSession creates an SFTP session over the given SSH client.
func NewSFTPSession(conn *ssh.Client) (*SFTPSession, error) {
	sftpClient, err := sftp.NewClient(conn)
	if err != nil {
		return nil, fmt.Errorf("sftp: open subsystem: %w", err)
	}
	return &SFTPSession{
		client: sftpClient,
		conn:   conn,
	}, nil
}

// NewSFTPSessionFromParams dials a fresh SSH connection and opens SFTP over it.
// This is used when there is no existing managed session to reuse.
func NewSFTPSessionFromParams(host string, port int, username, password, identityFile, proxyJump string) (*SFTPSession, error) {
	authMethods := buildSSHAuth(username, password, identityFile)
	if len(authMethods) == 0 {
		authMethods = defaultSSHAuth()
	}

	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         15 * time.Second,
	}

	var client *ssh.Client
	if proxyJump != "" {
		c, err := dialViaJump(proxyJump, host, port, cfg)
		if err != nil {
			return nil, err
		}
		client = c
	} else {
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		c, err := ssh.Dial("tcp", addr, cfg)
		if err != nil {
			return nil, fmt.Errorf("sftp: ssh dial: %w", err)
		}
		client = c
	}

	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("sftp: open subsystem: %w", err)
	}
	return &SFTPSession{
		client: sftpClient,
		conn:   client,
	}, nil
}

// dialViaJump dials the target through one or more jump hosts.
func dialViaJump(proxyJump, targetHost string, targetPort int, cfg *ssh.ClientConfig) (*ssh.Client, error) {
	hops := splitHosts(proxyJump)

	var currentClient *ssh.Client
	var jumpClients []*ssh.Client

	for _, hop := range hops {
		hopUser, hopHost, hopPort := parseJumpHost(hop)

		var conn net.Conn
		var err error
		if currentClient == nil {
			conn, err = net.DialTimeout("tcp", net.JoinHostPort(hopHost, hopPort), 15*time.Second)
		} else {
			conn, err = currentClient.Dial("tcp", net.JoinHostPort(hopHost, hopPort))
		}
		if err != nil {
			closeClients(jumpClients)
			return nil, fmt.Errorf("sftp: jump host %s: %w", hop, err)
		}

		hopAuth := buildSSHAuth(hopUser, "", "")
		if len(hopAuth) == 0 {
			hopAuth = defaultSSHAuth()
		}
		hopCfg := &ssh.ClientConfig{
			User:            hopUser,
			Auth:            hopAuth,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         15 * time.Second,
		}
		ncc, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(hopHost, hopPort), hopCfg)
		if err != nil {
			conn.Close()
			closeClients(jumpClients)
			return nil, fmt.Errorf("sftp: jump host SSH %s: %w", hop, err)
		}
		currentClient = ssh.NewClient(ncc, chans, reqs)
		jumpClients = append(jumpClients, currentClient)
	}

	// Final hop to target
	targetAddr := net.JoinHostPort(targetHost, strconv.Itoa(targetPort))
	var finalConn net.Conn
	var err error
	if currentClient == nil {
		finalConn, err = net.DialTimeout("tcp", targetAddr, 15*time.Second)
	} else {
		finalConn, err = currentClient.Dial("tcp", targetAddr)
	}
	if err != nil {
		closeClients(jumpClients)
		return nil, fmt.Errorf("sftp: target dial via jump: %w", err)
	}

	ncc, chans, reqs, err := ssh.NewClientConn(finalConn, targetAddr, cfg)
	if err != nil {
		finalConn.Close()
		closeClients(jumpClients)
		return nil, fmt.Errorf("sftp: target SSH via jump: %w", err)
	}
	return ssh.NewClient(ncc, chans, reqs), nil
}

func closeClients(clients []*ssh.Client) {
	for i := len(clients) - 1; i >= 0; i-- {
		clients[i].Close()
	}
}

func splitHosts(s string) []string {
	var result []string
	for _, h := range splitByComma(s) {
		if h != "" {
			result = append(result, h)
		}
	}
	return result
}

func splitByComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// List returns the contents of a remote directory, sorted dirs-first then
// alphabetically.
func (s *SFTPSession) List(path string) ([]FileEntry, error) {
	infos, err := s.client.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("sftp list %q: %w", path, err)
	}

	entries := make([]FileEntry, 0, len(infos))
	for _, fi := range infos {
		entries = append(entries, FileEntry{
			Name:    fi.Name(),
			Size:    fi.Size(),
			Mode:    fi.Mode(),
			ModTime: fi.ModTime(),
			IsDir:   fi.IsDir(),
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// Download copies a remote file to a local path.
func (s *SFTPSession) Download(remote, local string) error {
	src, err := s.client.Open(remote)
	if err != nil {
		return fmt.Errorf("sftp download open %q: %w", remote, err)
	}
	defer src.Close()

	dst, err := os.Create(local)
	if err != nil {
		return fmt.Errorf("sftp download create %q: %w", local, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("sftp download copy: %w", err)
	}
	return nil
}

// Upload copies a local file to a remote path.
func (s *SFTPSession) Upload(local, remote string) error {
	src, err := os.Open(local)
	if err != nil {
		return fmt.Errorf("sftp upload open %q: %w", local, err)
	}
	defer src.Close()

	dst, err := s.client.Create(remote)
	if err != nil {
		return fmt.Errorf("sftp upload create %q: %w", remote, err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("sftp upload copy: %w", err)
	}
	return nil
}

// Mkdir creates a remote directory.
func (s *SFTPSession) Mkdir(path string) error {
	if err := s.client.Mkdir(path); err != nil {
		return fmt.Errorf("sftp mkdir %q: %w", path, err)
	}
	return nil
}

// Remove deletes a remote file or empty directory.
func (s *SFTPSession) Remove(path string) error {
	if err := s.client.Remove(path); err != nil {
		return fmt.Errorf("sftp remove %q: %w", path, err)
	}
	return nil
}

// Getwd returns the remote current working directory.
func (s *SFTPSession) Getwd() (string, error) {
	return s.client.Getwd()
}

// Close closes the SFTP client. The underlying SSH connection is NOT closed
// here because it may be shared with a running ManagedSession.
func (s *SFTPSession) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// CloseAll closes both the SFTP client and the underlying SSH connection.
// Call this when the SSH connection was created exclusively for SFTP.
func (s *SFTPSession) CloseAll() error {
	err := s.Close()
	if s.conn != nil {
		s.conn.Close()
	}
	return err
}
