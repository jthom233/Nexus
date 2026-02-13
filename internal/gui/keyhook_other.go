//go:build !windows

package gui

type hookEvent struct {
	scancode uint16
	extended bool
	release  bool
}

func installKeyHook() {}

func setHookWindow(_ uintptr) {}

func drainHookEvents() []hookEvent { return nil }

func stopKeyHook() {}
