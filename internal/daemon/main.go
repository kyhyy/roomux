package daemon

import (
	"bufio"
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
		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			fmt.Println("roomux: received", scanner.Text())
			fmt.Fprintln(conn, "echo"+scanner.Text())
		}
		fmt.Println("roomux: closed connection")
		conn.Close()
	}
}
