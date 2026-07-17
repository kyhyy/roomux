package client

import (
	"fmt"
	"net"
	"os"
)

const socketPath = "/tmp/roomux.sock"

func Connect() {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to connect:", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Println("connected to daemon")
}
