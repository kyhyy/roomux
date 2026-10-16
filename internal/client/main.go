package client

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"roomux/internal/proto"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

const socketPath = "/tmp/roomux.sock"

// barKey is Ctrl+Space. It opens the roomux command bar. The NUL byte is the
// one key that no shell and no full-screen program needs, thus the bar takes no
// key from them. The client sends every other key to the shell.
const barKey = 0x00

// outMu guards os.Stdout. The pty output and the command bar both write there.
var outMu sync.Mutex

// writeOut writes to the terminal. It holds outMu, thus a bar line and a pty
// chunk do not mix in the middle of a write.
func writeOut(b []byte) {
	outMu.Lock()
	os.Stdout.Write(b)
	outMu.Unlock()
}

// Connect attaches to the daemon and stays in the shared session until the user
// detaches.
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

	fmt.Println("connected to daemon (Ctrl+Space opens the roomux command bar)")

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to make terminal raw:", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	sendSize(conn)
	go watchSize(conn)

	go keys(conn)

	// The pty output goes to the terminal. A read does not stop until the daemon
	// closes the connection, thus this loop keeps Connect alive.
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			writeOut(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// keys reads the keyboard and sends it to the daemon. Ctrl+Space opens the
// command bar; the bar consumes its own keys. Every other key goes to the
// shell.
func keys(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 4096)
	b := &bar{conn: conn}
	for {
		n, err := os.Stdin.Read(buf)

		out := make([]byte, 0, n)
		detach := false
		for _, key := range buf[:n] {
			if b.open {
				done, want := b.feed(key)
				if done {
					b.open = false
					detach = want
				}
				if detach {
					break
				}
				continue
			}
			if key == barKey {
				b.openBar()
				continue
			}
			out = append(out, key)
		}
		if len(out) > 0 {
			proto.Write(conn, proto.Input, out)
		}
		if detach || err != nil {
			return
		}
	}
}

// bar is the roomux command line. It is open while the user types a command.
type bar struct {
	conn net.Conn
	open bool
	line []byte
}

// openBar shows the command line and starts one command.
func (b *bar) openBar() {
	b.open = true
	b.line = b.line[:0]
	writeOut([]byte("\r\nroomux> "))
}

// feed handles one key of the command. It returns true when the command is
// finished, and reports a detach. A finished command runs.
func (b *bar) feed(key byte) (done, detach bool) {
	switch {
	case key == '\r' || key == '\n':
		writeOut([]byte("\r\n"))
		return true, b.run()
	case key == 0x1b || key == 0x03: // Esc or Ctrl+C
		writeOut([]byte("\r\n"))
		return true, false
	case key == 0x7f || key == 0x08: // Backspace
		if len(b.line) > 0 {
			b.line = b.line[:len(b.line)-1]
			writeOut([]byte("\b \b"))
		}
	case key >= 0x20 && key < 0x7f:
		b.line = append(b.line, key)
		writeOut([]byte{key})
	}
	return false, false
}

// run runs the command on the line. It returns true for detach.
func (b *bar) run() bool {
	fields := strings.Fields(string(b.line))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "new":
		proto.Write(b.conn, proto.NewWindow, nil)
		clearScreen()
	case "next":
		proto.Write(b.conn, proto.CycleWindow, proto.DeltaPayload(1))
		clearScreen()
	case "prev":
		proto.Write(b.conn, proto.CycleWindow, proto.DeltaPayload(-1))
		clearScreen()
	case "select":
		if len(fields) == 2 {
			if index, err := strconv.Atoi(fields[1]); err == nil && index >= 0 {
				proto.Write(b.conn, proto.SelectWindow, proto.IndexPayload(uint16(index)))
				clearScreen()
			}
		}
	case "detach":
		return true
	default:
		writeOut([]byte("\r\nroomux: unknown command: " + fields[0] + "\r\n"))
	}
	return false
}

// clearScreen wipes the terminal after a window change, because the daemon has
// no screen buffer to draw again.
func clearScreen() {
	writeOut([]byte("\x1b[2J\x1b[H"))
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
