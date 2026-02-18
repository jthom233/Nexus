//go:build !windows

package gui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// detectImageClipboard reports whether the local system clipboard currently
// holds image data (image/png format). It checks Wayland first, then X11.
// Returns false silently on any error.
func detectImageClipboard() bool {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return detectImageWayland()
	}
	return detectImageX11()
}

func detectImageWayland() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wl-paste", "--list-types").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "image/png")
}

func detectImageX11() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xclip", "-o", "-selection", "clipboard", "-t", "TARGETS").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "image/png")
}

// readImageClipboard reads the current clipboard image as raw PNG bytes.
// It supports Wayland (via wl-paste) and X11 (via xclip).
func readImageClipboard() ([]byte, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return readImageWayland()
	}
	return readImageX11()
}

func readImageWayland() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "wl-paste", "--no-newline", "--type", "image/png").Output()
	if err != nil {
		return nil, fmt.Errorf("clipboard_image: wl-paste failed: %w", err)
	}
	return out, nil
}

func readImageX11() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "xclip", "-o", "-selection", "clipboard", "-t", "image/png").Output()
	if err != nil {
		return nil, fmt.Errorf("clipboard_image: xclip read failed: %w", err)
	}
	return out, nil
}

// writeImageClipboard writes raw PNG bytes to the local system clipboard.
// It supports Wayland (via wl-copy) and X11 (via xclip).
func writeImageClipboard(pngData []byte) error {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return writeImageWayland(pngData)
	}
	return writeImageX11(pngData)
}

func writeImageWayland(pngData []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wl-copy", "--type", "image/png")
	cmd.Stdin = bytes.NewReader(pngData)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard_image: wl-copy failed: %w", err)
	}
	return nil
}

func writeImageX11(pngData []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xclip", "-i", "-selection", "clipboard", "-t", "image/png")
	cmd.Stdin = bytes.NewReader(pngData)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clipboard_image: xclip write failed: %w", err)
	}
	return nil
}
