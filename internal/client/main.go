package client

import (
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/term"
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

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to make terminal raw:", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				if buf[0] == 4 {
					break
				}
				conn.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		conn.Close()
	}()
	io.Copy(os.Stdout, conn)
}
