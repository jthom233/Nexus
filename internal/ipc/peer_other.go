//go:build !linux

package ipc

import "net"

func verifyPeer(conn net.Conn) error {
	return nil
}
