package client

import (
	"bufio"
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

	fmt.Println("connected to daemon (type something, Enter to send, Ctrl+D to quit)")

	stdin := bufio.NewScanner(os.Stdin)
	server := bufio.NewScanner(conn)

	for stdin.Scan() {
		line := stdin.Text()

		fmt.Fprintln(conn, line)
		if !server.Scan() {
			fmt.Println("daemon closed the connection")
			break
		}
		fmt.Println(server.Text())
	}
}
