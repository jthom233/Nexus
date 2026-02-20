package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/dr4zz/nexus/internal/config"
	"github.com/dr4zz/nexus/internal/launcher"
	"github.com/dr4zz/nexus/internal/termcap"
	"github.com/dr4zz/nexus/internal/theme"
	"github.com/dr4zz/nexus/internal/tui"
	"github.com/dr4zz/nexus/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

func main() {
	cfg, isNew, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	var masterPassword []byte
	if isNew {
		masterPassword, err = firstLaunchSetup(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	if masterPassword == nil {
		masterPassword, err = handleMasterPassword(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	v, err := vault.Open(vault.Backend(cfg.Settings.Vault), string(masterPassword))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open credential vault: %v\n", err)
		v = nil
	}

	// Detect terminal capabilities
	caps := termcap.Detect()

	// Initialize clipboard based on capabilities
	termcap.InitClipboard(caps)

	// Apply color profile
	profileOverride := cfg.Settings.ColorProfileOverride()
	if profileOverride != "auto" {
		theme.SetProfile(profileOverride)
	} else {
		theme.SetProfile(caps.ColorProfile)
	}

	// Apply theme from config
	if cfg.Settings.Theme != "" && cfg.Settings.Theme != "default" {
		theme.Set(cfg.Settings.Theme)
	}

	opts := []tea.ProgramOption{
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	}

	p := tea.NewProgram(
		tui.NewApp(cfg, v),
		opts...,
	)

	// Wire the program reference into the launcher package so that the
	// RDPLauncher can forward GUI log lines into the Bubbletea event loop.
	launcher.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func firstLaunchSetup(cfg *config.Config) ([]byte, error) {
	fmt.Println()
	fmt.Println("  Welcome to Nexus — Terminal Connection Manager")
	fmt.Println()

	// Offer SSH config import
	sshConns, err := config.ImportSSHConfig()
	if err == nil && len(sshConns) > 0 {
		fmt.Printf("  Found %d hosts in ~/.ssh/config.\n", len(sshConns))
		fmt.Print("  Import them? [Y/n] ")
		answer := readLine()
		if answer == "" || answer == "y" || answer == "Y" {
			cfg.Groups = []config.Group{
				{Name: "imported", Color: "#5f87af"},
			}
			for i := range sshConns {
				sshConns[i].Group = "imported"
			}
			cfg.Connections = append(sshConns, cfg.Connections...)
			fmt.Printf("  Imported %d connections.\n", len(sshConns))
		}
	} else {
		// SSH config absent or unreadable — start with sample connections
		sample := config.SampleConfig()
		cfg.Groups = sample.Groups
		cfg.Connections = sample.Connections
	}

	fmt.Println()

	// Optional master password
	fmt.Println("  Set a master password to encrypt stored credentials.")
	fmt.Println("  Press Enter to skip (you can set one later).")
	fmt.Println()

	var masterPassword []byte
	for {
		fmt.Print("  Master password: ")
		pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}

		if len(pw1) == 0 {
			fmt.Println("  Skipped — no master password set.")
			break
		}

		fmt.Print("  Confirm: ")
		pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}

		if !bytes.Equal(pw1, pw2) {
			fmt.Println("  Passwords do not match — try again.")
			fmt.Println()
			continue
		}

		cfg.SetKey(pw1)
		if err := cfg.SetPasswordVerify(pw1); err != nil {
			return nil, err
		}

		fmt.Println("  Master password set.")
		masterPassword = pw1
		break
	}

	// Save the config to disk
	if err := config.Save(cfg); err != nil {
		return nil, fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Config saved to %s\n", config.ConfigPath())
	fmt.Println("  Edit it to customize, or use :add/:edit within the app.")
	fmt.Println()

	return masterPassword, nil
}

func handleMasterPassword(cfg *config.Config) ([]byte, error) {
	// If a master password was configured, always prompt for it
	if cfg.HasMasterPassword() {
		// Allow env var for scripting/testing (exposes password in /proc — testing only)
		if envPw := os.Getenv("MASTER_PASSWORD"); envPw != "" {
			if err := cfg.VerifyMasterPassword([]byte(envPw)); err != nil {
				return nil, err
			}
			cfg.SetKey([]byte(envPw))
			if err := cfg.DecryptPasswords(); err != nil {
				return nil, fmt.Errorf("decryption failed: %w", err)
			}
			return []byte(envPw), nil
		}

		fmt.Print("Master password: ")
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}

		if err := cfg.VerifyMasterPassword(pw); err != nil {
			return nil, err
		}

		cfg.SetKey(pw)
		if err := cfg.DecryptPasswords(); err != nil {
			return nil, fmt.Errorf("decryption failed: %w", err)
		}
		return pw, nil
	}

	// Legacy path: no password_verify but has passwords in config
	if !cfg.HasAnyPasswords() {
		return nil, nil
	}

	hasEncrypted := cfg.HasEncryptedPasswords()
	hasPlaintext := cfg.HasPlaintextPasswords()

	var password []byte

	if hasEncrypted {
		// Allow env var for scripting/testing
		if envPw := os.Getenv("MASTER_PASSWORD"); envPw != "" {
			password = []byte(envPw)
		} else {
			fmt.Print("Master password: ")
			pw, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				return nil, fmt.Errorf("reading password: %w", err)
			}
			password = pw
		}
	} else if hasPlaintext {
		// Only plaintext passwords — set up encryption
		fmt.Print("Set a master password to encrypt credentials: ")
		pw1, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}

		fmt.Print("Confirm: ")
		pw2, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}

		if !bytes.Equal(pw1, pw2) {
			return nil, fmt.Errorf("passwords do not match")
		}

		if len(pw1) == 0 {
			return nil, fmt.Errorf("password cannot be empty")
		}

		password = pw1
	} else {
		return nil, nil
	}

	// Store raw password as key — Encrypt/Decrypt handle per-password key derivation
	cfg.SetKey(password)

	// Decrypt all passwords in memory
	if err := cfg.DecryptPasswords(); err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	// If migrating from plaintext, save immediately to encrypt on disk
	if hasPlaintext && !hasEncrypted {
		// Also set up password verify sentinel for future launches
		if err := cfg.SetPasswordVerify(password); err != nil {
			return nil, fmt.Errorf("setting password verify: %w", err)
		}
		if err := config.Save(cfg); err != nil {
			return nil, fmt.Errorf("saving encrypted config: %w", err)
		}
		fmt.Println("Passwords encrypted successfully.")
	}

	return password, nil
}

// readLine reads a single line from stdin (for y/n prompts).
func readLine() string {
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}
