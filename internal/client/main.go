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

// outMu guards os.Stdout. The pty output, the status line, and the command bar
// all write to the terminal.
var outMu sync.Mutex

// bottom holds the state of the last terminal row. The client keeps that row for
// the status line and the command bar, and it tells the daemon that the programs
// have one row less.
var bottom struct {
	rows   int // the real rows of the terminal
	cols   int
	status string
	open   bool
	line   []byte
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
	defer func() {
		outMu.Lock()
		resetTermLocked()
		outMu.Unlock()
		term.Restore(int(os.Stdin.Fd()), oldState)
	}()

	go watchSize(conn)
	go keys(conn)

	// Keep the bottom row, then tell the daemon the smaller size.
	resize(conn)

	// The daemon sends frames: pty output for this window, and status lines.
	for {
		kind, payload, err := proto.Read(conn)
		if err != nil {
			return
		}
		outMu.Lock()
		switch kind {
		case proto.Output:
			os.Stdout.Write(payload)
			// An erase clears the whole screen, and thus it also clears the
			// bottom row. Draw that row again.
			if hasErase(payload) {
				drawBottomLocked()
			}
		case proto.Status:
			bottom.status = string(payload)
			if !bottom.open {
				drawBottomLocked()
			}
		}
		outMu.Unlock()
	}
}

// keys reads the keyboard and sends it to the daemon. Ctrl+Space opens the
// command bar; the bar consumes its own keys. Every other key goes to the shell.
func keys(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 4096)
	b := &bar{conn: conn}
	for {
		n, err := os.Stdin.Read(buf)

		out := make([]byte, 0, n)
		detach := false
		for _, key := range buf[:n] {
			if bottom.open {
				if b.feed(key) {
					detach = b.detach
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

// bar reads one roomux command and runs it. The command line is drawn on the
// bottom row.
type bar struct {
	conn   net.Conn
	detach bool
}

// openBar starts one command.
func (b *bar) openBar() {
	outMu.Lock()
	bottom.open = true
	bottom.line = bottom.line[:0]
	drawBottomLocked()
	outMu.Unlock()
}

// feed handles one key. It returns true when the command is finished.
func (b *bar) feed(key byte) bool {
	outMu.Lock()
	defer outMu.Unlock()

	switch {
	case key == '\r' || key == '\n':
		line := string(bottom.line)
		bottom.open = false
		bottom.line = bottom.line[:0]
		b.runLocked(line)
		drawBottomLocked() // show the status line again
		return true
	case key == 0x1b || key == 0x03: // Esc or Ctrl+C
		bottom.open = false
		bottom.line = bottom.line[:0]
		drawBottomLocked()
		return true
	case key == 0x7f || key == 0x08: // Backspace
		if len(bottom.line) > 0 {
			bottom.line = bottom.line[:len(bottom.line)-1]
		}
		drawBottomLocked()
	case key >= 0x20 && key < 0x7f:
		bottom.line = append(bottom.line, key)
		drawBottomLocked()
	}
	return false
}

// runLocked runs the command on the line. The caller must hold outMu.
func (b *bar) runLocked(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}
	switch fields[0] {
	case "new":
		proto.Write(b.conn, proto.NewWindow, nil)
		clearScreenLocked()
	case "next":
		proto.Write(b.conn, proto.CycleWindow, proto.DeltaPayload(1))
		clearScreenLocked()
	case "prev":
		proto.Write(b.conn, proto.CycleWindow, proto.DeltaPayload(-1))
		clearScreenLocked()
	case "select":
		if len(fields) == 2 {
			if n, err := strconv.Atoi(fields[1]); err == nil && n >= 1 {
				proto.Write(b.conn, proto.SelectWindow, proto.IndexPayload(uint16(n-1)))
				clearScreenLocked()
			}
		}
	case "detach":
		b.detach = true
	default:
		os.Stdout.WriteString("\r\nroomux: unknown command: " + fields[0] + "\r\n")
	}
}

// clearScreenLocked wipes the terminal after a window change, because the daemon
// has no screen buffer to draw again. The caller must hold outMu.
func clearScreenLocked() {
	os.Stdout.WriteString("\x1b[2J\x1b[H")
	drawBottomLocked() // the wipe also cleared the bottom row
}

// drawBottomLocked draws the bottom row: the command line while the user types a
// command, or the status line otherwise. The caller must hold outMu.
func drawBottomLocked() {
	if bottom.rows < 2 || bottom.cols < 1 {
		return
	}

	text := bottom.status
	if bottom.open {
		text = ": " + string(bottom.line)
	}

	var out strings.Builder
	if !bottom.open {
		out.WriteString("\x1b7") // save the cursor of the programs
	}
	fmt.Fprintf(&out, "\x1b[%d;1H", bottom.rows)
	out.WriteString("\x1b[7m") // reverse video
	out.WriteString(fit(text, bottom.cols))
	out.WriteString("\x1b[0m")
	if bottom.open {
		// Leave the cursor at the end of the typed text, on the bottom row.
		col := len([]rune(text)) + 1
		if col > bottom.cols {
			col = bottom.cols
		}
		fmt.Fprintf(&out, "\x1b[%d;%dH", bottom.rows, col)
	} else {
		out.WriteString("\x1b8") // put the cursor of the programs back
	}
	os.Stdout.WriteString(out.String())
}

// fit cuts text to the width, and pads it with spaces to that width.
func fit(text string, width int) string {
	r := []rune(text)
	if len(r) > width {
		r = r[:width]
	}
	return string(r) + strings.Repeat(" ", width-len(r))
}

// hasErase reports whether the bytes hold an erase-in-display sequence
// (ESC [ ... J). That sequence clears the whole screen, and thus it clears the
// bottom row too.
func hasErase(b []byte) bool {
	for i := 0; i+1 < len(b); i++ {
		if b[i] != 0x1b || b[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < len(b) && b[j] >= '0' && b[j] <= '9' {
			j++
		}
		if j < len(b) && b[j] == 'J' {
			return true
		}
	}
	return false
}

// resize reads the local size, keeps the bottom row for the status line, tells
// the daemon the smaller size, and draws the bottom row. It runs at attach and
// after each SIGWINCH signal.
func resize(conn net.Conn) {
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}

	outMu.Lock()
	bottom.rows = rows
	bottom.cols = cols
	if rows > 1 {
		// The scroll region stops at the row above the status line, thus the
		// programs never draw on that line and never scroll it.
		os.Stdout.WriteString(fmt.Sprintf("\x1b7\x1b[1;%dr\x1b8", rows-1))
	}
	drawBottomLocked()
	outMu.Unlock()

	programRows := rows
	if programRows > 1 {
		programRows--
	}
	proto.Write(conn, proto.Resize, proto.ResizePayload(uint16(programRows), uint16(cols)))
}

// resetTermLocked drops the scroll region and clears the bottom row, so the
// terminal is clean after the client stops. The caller must hold outMu.
func resetTermLocked() {
	if bottom.rows < 2 {
		return
	}
	os.Stdout.WriteString("\x1b[r") // the whole screen scrolls again
	fmt.Fprintf(os.Stdout, "\x1b[%d;1H\x1b[2K", bottom.rows)
}

// watchSize resends the size after each SIGWINCH signal.
func watchSize(conn net.Conn) {
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	for range winch {
		resize(conn)
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
