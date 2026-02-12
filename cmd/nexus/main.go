package main

import (
	"bytes"
	"fmt"
	"os"
	"syscall"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := handleMasterPassword(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		tui.NewApp(cfg),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func handleMasterPassword(cfg *config.Config) error {
	if !cfg.HasAnyPasswords() {
		return nil
	}

	hasEncrypted := cfg.HasEncryptedPasswords()
	hasPlaintext := cfg.HasPlaintextPasswords()

	var password []byte

	if hasEncrypted {
		// Existing encrypted passwords — prompt for master password
		fmt.Print("Master password: ")
		pw, err := term.ReadPassword(syscall.Stdin)
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}
		password = pw
	} else if hasPlaintext {
		// Only plaintext passwords — set up encryption
		fmt.Print("Set a master password to encrypt credentials: ")
		pw1, err := term.ReadPassword(syscall.Stdin)
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}

		fmt.Print("Confirm: ")
		pw2, err := term.ReadPassword(syscall.Stdin)
		fmt.Println()
		if err != nil {
			return fmt.Errorf("reading password: %w", err)
		}

		if !bytes.Equal(pw1, pw2) {
			return fmt.Errorf("passwords do not match")
		}

		if len(pw1) == 0 {
			return fmt.Errorf("password cannot be empty")
		}

		password = pw1
	} else {
		return nil
	}

	// Store raw password as key — Encrypt/Decrypt handle per-password key derivation
	cfg.SetKey(password)

	// Decrypt all passwords in memory
	if err := cfg.DecryptPasswords(); err != nil {
		return fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	// If migrating from plaintext, save immediately to encrypt on disk
	if hasPlaintext && !hasEncrypted {
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("saving encrypted config: %w", err)
		}
		fmt.Println("Passwords encrypted successfully.")
	}

	return nil
}
