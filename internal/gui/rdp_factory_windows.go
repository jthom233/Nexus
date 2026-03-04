//go:build windows

package gui

// NewPlatformRDPSession returns a Windows-native MsTscSession that embeds the
// Microsoft Terminal Services ActiveX control (MsTscAx) into the Ebiten window.
// On Windows this is preferred over the pure-Go grdp fallback (RDPSession)
// because it uses the OS-native RDP client with full protocol support,
// GPU-accelerated rendering, NLA/CredSSP authentication, and clipboard integration.
func NewPlatformRDPSession(host string, port int, username, password, domain string, options map[string]interface{}) Session {
	return NewMsTscSession(host, port, username, password, domain, options)
}
