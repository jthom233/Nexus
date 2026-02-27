package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// buildSSHAuth builds authentication methods from explicit credentials.
// When an identity file is set, the password is also tried as a passphrase
// for encrypted private keys.
func buildSSHAuth(username, password, identityFile string) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	if identityFile != "" {
		if signer := loadSSHKey(identityFile, password); signer != nil {
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
// All available default keys are offered (no early exit on first success).
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
		filepath.Join(home, ".ssh", "id_dsa"),
	}

	for _, kf := range keyFiles {
		if signer := loadSSHKey(kf, ""); signer != nil {
			methods = append(methods, ssh.PublicKeys(signer))
		}
	}

	return methods
}

// loadSSHKey loads and parses a private SSH key from disk.
// If passphrase is non-empty and the key is encrypted, it is used to decrypt
// the key. Unencrypted keys are loaded regardless of the passphrase value.
func loadSSHKey(path, passphrase string) ssh.Signer {
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
	if err == nil {
		return signer
	}

	// If the key requires a passphrase and we have one, try with it.
	var missingErr *ssh.PassphraseMissingError
	if errors.As(err, &missingErr) && passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
		if err == nil {
			return signer
		}
	}

	return nil
}
