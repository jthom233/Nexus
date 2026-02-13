package main

import (
	"fmt"
	"os"

	"github.com/dr4zz/nexus/internal/gui"
	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	app := gui.NewApp()

	// Start IPC server
	ipcMgr, err := gui.NewIPCManager(app)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start IPC server: %v\n", err)
		os.Exit(1)
	}
	app.SetIPCManager(ipcMgr)
	defer ipcMgr.Close()

	// Run IPC server in background
	go func() {
		if err := ipcMgr.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "IPC server error: %v\n", err)
		}
	}()

	// Configure and run Ebiten window
	ebiten.SetWindowSize(1280, 800)
	ebiten.SetWindowTitle("Nexus - Remote Sessions")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetTPS(30) // 30 updates per second

	if err := ebiten.RunGame(app); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
