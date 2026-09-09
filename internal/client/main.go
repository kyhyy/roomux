package client

import (
	"fmt"
	"io"
	"net"
	"os"

	"golang.org/x/term"
)

const socketPath = "/tmp/roomux.sock"

// Detach is Ctrl+B then d. Unprefixed bytes are forwarded untouched, so Ctrl+D
// reaches the shell as a real EOF. Ctrl+B twice sends one literal Ctrl+B; any
// other key after the prefix is not a command and is dropped, as in tmux.
const (
	prefixKey = 0x02 // Ctrl+B
	detachKey = 'd'
)

func Connect() {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to connect:", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Println("connected to daemon (Ctrl+B d to detach)")

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to make terminal raw:", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	go func() {
		defer conn.Close()

		buf := make([]byte, 4096)
		out := make([]byte, 0, len(buf))
		prefixed := false // survives across reads: the two keys can arrive apart

		for {
			n, err := os.Stdin.Read(buf)

			out = out[:0]
			detach := false
			for _, b := range buf[:n] {
				if prefixed {
					prefixed = false
					switch b {
					case detachKey:
						detach = true
					case prefixKey:
						out = append(out, prefixKey)
					}
					// Any other key after the prefix is not a command; drop it.
					if detach {
						break
					}
					continue
				}
				if b == prefixKey {
					prefixed = true
					continue
				}
				out = append(out, b)
			}
			if len(out) > 0 {
				conn.Write(out)
			}
			if detach || err != nil {
				return
			}
		}
	}()

	io.Copy(os.Stdout, conn)
}
