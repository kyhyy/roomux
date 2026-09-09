package daemon

import (
	"fmt"
	"io"
	"net"
	"os"
	"roomux/internal/pty"
	"sync"
)

const socketPath = "/tmp/roomux.sock"

// The pty master has exactly one reader (broadcast), which writes to whichever
// client is attached right now. mu guards attached; nil means nobody is
// attached and pty output is dropped.
var (
	mu       sync.Mutex
	attached net.Conn
)

func Run() {
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to listen on socket:", err)
		os.Exit(1)
	}
	defer ln.Close()

	ptmx, err := pty.Spawn_pty()
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to spawn pty:", err)
		os.Exit(1)
	}
	defer func() {
		_ = ptmx.Close()
	}()

	go broadcast(ptmx)

	fmt.Println("running roomux daemon on", socketPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to accept connection:", err)
			continue
		}
		fmt.Println("roomux: accepted connection")

		go serve(ptmx, conn)
	}
}

// broadcast is the daemon's single permanent reader of the pty master. It must
// outlive every client: a goroutine blocked in ptmx.Read cannot be interrupted
// without closing the pty, which would kill the shell. So the reader stays put
// and the destination changes underneath it.
func broadcast(ptmx *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			mu.Lock()
			if attached != nil {
				if _, werr := attached.Write(buf[:n]); werr != nil {
					attached = nil
				}
			}
			mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// serve makes conn the attached client and forwards its input to the shell
// until it detaches. Only one client is supported at this stage, so a new
// attach evicts the previous one instead of silently starving it.
func serve(ptmx *os.File, conn net.Conn) {
	mu.Lock()
	if attached != nil {
		attached.Close()
	}
	attached = conn
	mu.Unlock()

	_, _ = io.Copy(ptmx, conn)

	mu.Lock()
	if attached == conn {
		attached = nil
	}
	mu.Unlock()

	conn.Close()
	fmt.Println("roomux: closed connection")
}
