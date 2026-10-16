package client

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"roomux/internal/proto"
	"syscall"

	"golang.org/x/term"
)

const socketPath = "/tmp/roomux.sock"

// Detach is Ctrl+B then d. The client sends all other keys to the shell, thus
// the shell gets a true Ctrl+D. Ctrl+B twice sends one Ctrl+B. The client drops
// any other key after the prefix.
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

	if err := proto.Write(conn, proto.Attach, nil); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to attach:", err)
		os.Exit(1)
	}

	fmt.Println("connected to daemon (Ctrl+B d to detach)")

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to make terminal raw:", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	sendSize(conn)
	go watchSize(conn)

	go func() {
		defer conn.Close()

		buf := make([]byte, 4096)
		out := make([]byte, 0, len(buf))
		// prefixed stays true between reads. The two keys can come apart.
		prefixed := false

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
				proto.Write(conn, proto.Input, out)
			}
			if detach || err != nil {
				return
			}
		}
	}()

	io.Copy(os.Stdout, conn)
}

// sendSize tells the daemon the size of the local terminal.
func sendSize(conn net.Conn) {
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	proto.Write(conn, proto.Resize, proto.ResizePayload(uint16(rows), uint16(cols)))
}

// watchSize sends the new size after each SIGWINCH signal.
func watchSize(conn net.Conn) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	for range winch {
		sendSize(conn)
	}
}

// Who asks the daemon which clients are attached and prints the answer.
func Who() {
	ask(proto.Who)
}

// History asks the daemon for the history log and prints it.
func History() {
	ask(proto.History)
}

// ask opens a short connection, sends one opening frame, and prints the reply.
func ask(kind byte) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to connect:", err)
		os.Exit(1)
	}
	defer conn.Close()

	if err := proto.Write(conn, kind, nil); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to send the request:", err)
		os.Exit(1)
	}
	io.Copy(os.Stdout, conn)
}
