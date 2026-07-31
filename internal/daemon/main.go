package daemon

import (
	"fmt"
	"net"
	"os"
	"roomux/internal/pty"
)

const socketPath = "/tmp/roomux.sock"

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

	fmt.Println("running roomux daemon on", socketPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to accept connection:", err)
			continue
		}
		fmt.Println("roomux: accepted connection")

		go func() {
			pty.Attach(ptmx, conn)
			fmt.Println("roomux: closed connection")
			conn.Close()
		}()
	}
}
