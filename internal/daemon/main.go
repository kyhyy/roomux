package daemon

import (
	"fmt"
	"net"
	"os"
	"roomux/internal/proto"
	"roomux/internal/pty"
	"sync"
)

const socketPath = "/tmp/roomux.sock"

// A read of the pty does not stop. Therefore one goroutine reads the pty for
// the life of the daemon and writes to the client in this slot. mu protects
// the slot. If the slot is nil, no client is attached and the daemon discards
// the output.
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

// serve makes conn the attached client and reads its frames. A new client
// replaces the previous client, because the daemon holds only one slot.
func serve(ptmx *os.File, conn net.Conn) {
	mu.Lock()
	if attached != nil {
		attached.Close()
	}
	attached = conn
	mu.Unlock()

	// The label is necessary. A plain break in a switch does not stop the loop.
loop:
	for {
		kind, payload, err := proto.Read(conn)
		if err != nil {
			break
		}

		switch kind {
		case proto.Input:
			if _, err := ptmx.Write(payload); err != nil {
				break loop
			}
		case proto.Resize:
			rows, cols, err := proto.ParseResize(payload)
			if err != nil {
				fmt.Fprintln(os.Stderr, "roomux:", err)
				continue
			}
			if err := pty.Setsize(ptmx, rows, cols); err != nil {
				fmt.Fprintln(os.Stderr, "roomux: failed to set pty size:", err)
			}
		}
	}

	mu.Lock()
	if attached == conn {
		attached = nil
	}
	mu.Unlock()

	conn.Close()
	fmt.Println("roomux: closed connection")
}
