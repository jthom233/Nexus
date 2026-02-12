package session

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// buildSSHAuth builds authentication methods from explicit credentials.
func buildSSHAuth(username, password, identityFile string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	if identityFile != "" {
		if signer := loadSSHKey(identityFile); signer != nil {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	if password != "" {
		methods = append(methods, ssh.Password(password))
		methods = append(methods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = password
				}
				return answers, nil
			},
		))
	}

	return methods
}

// defaultSSHAuth returns auth methods using default SSH key locations.
func defaultSSHAuth() []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	home, err := os.UserHomeDir()
	if err != nil {
		return methods
	}

	keyFiles := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}

	for _, kf := range keyFiles {
		if signer := loadSSHKey(kf); signer != nil {
			methods = append(methods, ssh.PublicKeys(signer))
			break
		}
	}

	return methods
}

// loadSSHKey loads and parses a private SSH key from disk.
func loadSSHKey(path string) ssh.Signer {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	signer, err := ssh.ParsePrivateKey(keyData)
	if err != nil {
		return nil
	}

	return signer
}
