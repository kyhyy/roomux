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

type size struct {
	rows uint16
	cols uint16
}

// A read of the pty does not stop. Therefore one goroutine reads the pty for
// the life of the daemon and writes to every attached client. mu protects the
// set. An empty set means that the daemon discards the output.
var (
	mu      sync.Mutex
	clients = make(map[net.Conn]size)
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

// broadcast reads the pty and writes to every attached client.
func broadcast(ptmx *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			mu.Lock()
			for conn := range clients {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					delete(clients, conn)
				}
			}
			mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// applySize gives the pty the smallest size of the attached clients. A larger
// client then has an unused margin, but no client sees a broken screen. The
// caller must hold mu. A client that sent no size has a zero size, thus
// applySize ignores it.
func applySize(ptmx *os.File) {
	var min size
	for _, s := range clients {
		if s.rows == 0 || s.cols == 0 {
			continue
		}
		if min.rows == 0 || s.rows < min.rows {
			min.rows = s.rows
		}
		if min.cols == 0 || s.cols < min.cols {
			min.cols = s.cols
		}
	}
	if min.rows == 0 || min.cols == 0 {
		return
	}
	if err := pty.Setsize(ptmx, min.rows, min.cols); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to set pty size:", err)
	}
}

// serve adds conn to the set of clients and reads its frames. Every client
// gets the same output, thus more than one client can watch one shell.
func serve(ptmx *os.File, conn net.Conn) {
	mu.Lock()
	clients[conn] = size{}
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
			mu.Lock()
			clients[conn] = size{rows: rows, cols: cols}
			applySize(ptmx)
			mu.Unlock()
		}
	}

	mu.Lock()
	delete(clients, conn)
	applySize(ptmx)
	mu.Unlock()

	conn.Close()
	fmt.Println("roomux: closed connection")
}
