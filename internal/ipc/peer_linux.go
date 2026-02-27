//go:build linux

package ipc

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

func verifyPeer(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return nil // not a Unix connection, skip check
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return fmt.Errorf("ipc: syscall conn: %w", err)
	}
	var cred *unix.Ucred
	var credErr error
	err = raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return fmt.Errorf("ipc: control: %w", err)
	}
	if credErr != nil {
		return fmt.Errorf("ipc: get peer cred: %w", credErr)
	}
	if cred.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("ipc: rejected connection from uid %d (expected %d)", cred.Uid, os.Getuid())
	}
	return nil
}
