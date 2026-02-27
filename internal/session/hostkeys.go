package session

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var hostKeyMu sync.Mutex

// HostKeyCallback returns an ssh.HostKeyCallback that implements TOFU.
// It checks ~/.ssh/known_hosts (if it exists) and ~/.config/nexus/known_hosts.
// Unknown hosts are automatically added to the Nexus known_hosts file.
// Changed keys are rejected with an error.
func HostKeyCallback() ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		nexusPath := nexusKnownHostsPath()
		sysPath := systemKnownHostsPath()

		// Build list of known_hosts files to check.
		var files []string
		if sysPath != "" {
			if _, err := os.Stat(sysPath); err == nil {
				files = append(files, sysPath)
			}
		}
		if nexusPath != "" {
			if _, err := os.Stat(nexusPath); err == nil {
				files = append(files, nexusPath)
			}
		}

		// If we have known_hosts files, check them.
		if len(files) > 0 {
			checker, err := knownhosts.New(files...)
			if err == nil {
				err = checker(hostname, remote, key)
				if err == nil {
					return nil // Key matches.
				}
				// Check if it's a key-changed error.
				var keyErr *knownhosts.KeyError
				if errors.As(err, &keyErr) && len(keyErr.Want) > 0 {
					return fmt.Errorf(
						"WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED for %s!\n"+
							"Someone could be eavesdropping on you (man-in-the-middle attack).\n"+
							"The %s host key fingerprint is %s.\n"+
							"To fix this, remove the offending entry from:\n  %s",
						hostname,
						key.Type(),
						ssh.FingerprintSHA256(key),
						nexusPath,
					)
				}
				// Key not found — fall through to TOFU.
			}
		}

		// TOFU: Add the key to the Nexus known_hosts file.
		return addHostKey(nexusPath, hostname, remote, key)
	}
}

func addHostKey(path string, hostname string, remote net.Addr, key ssh.PublicKey) error {
	if path == "" {
		return nil
	}
	hostKeyMu.Lock()
	defer hostKeyMu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating known_hosts dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("opening known_hosts: %w", err)
	}
	defer f.Close()

	addr := knownhosts.Normalize(hostname)
	line := knownhosts.Line([]string{addr}, key)
	_, err = fmt.Fprintln(f, line)
	return err
}

func nexusKnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "nexus", "known_hosts")
}

func systemKnownHostsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}
