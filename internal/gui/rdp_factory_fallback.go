//go:build !(windows || (linux && cgo))
// +build !windows,!linux !cgo

package gui

// NewPlatformRDPSession creates an RDP session using the pure-Go grdp library.
// This is the fallback for platforms without a native RDP backend.
func NewPlatformRDPSession(host string, port int, username, password, domain string, options map[string]interface{}) Session {
	return NewRDPSession(host, port, username, password, domain, options)
}
