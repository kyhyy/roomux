package daemon

import (
	"fmt"
	"net"
	"os"
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

	fmt.Println("running roomux daemon on", socketPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to accept connection:", err)
			continue
		}
		fmt.Println("roomux: accepted connection")
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				conn.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		fmt.Println("roomux: closed connection")
		conn.Close()
	}
}
