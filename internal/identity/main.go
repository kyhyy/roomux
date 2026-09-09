package identity

import (
	"errors"
	"net"

	"golang.org/x/sys/unix"
)

// Peer asks the kernel for the credentials of the process at the other end of
// conn.
func Peer(conn net.Conn) (*unix.Ucred, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("identity: connection is not a unix socket")
	}

	raw, err := unixConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	// Control runs the function on the real file descriptor of the socket.
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, credErr
}
