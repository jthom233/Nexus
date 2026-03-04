//go:build linux && cgo

package gui

// NewPlatformRDPSession creates an RDP session using FreeRDP via CGo.
// This provides full codec support (H.264/GFX, RemoteFX, NSCodec) on Linux.
func NewPlatformRDPSession(host string, port int, username, password, domain string, options map[string]interface{}) Session {
	return NewFreeRDPSession(host, port, username, password, domain, options)
}
