package daemon

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"roomux/internal/identity"
	"roomux/internal/proto"
	"roomux/internal/pty"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const socketPath = "/tmp/roomux.sock"

type size struct {
	rows uint16
	cols uint16
}

// outBuffer is the number of pty chunks that wait for one client. One chunk is
// at most 4096 bytes, thus 32 chunks are about 128 KiB.
const outBuffer = 32

// dropAfter is how long the daemon waits for a client to accept a chunk before
// it drops that client. A client that reads drains much faster than this. A
// client that stopped reading fills its buffer and is gone after this time.
const dropAfter = 2 * time.Second

// The kernel gives the daemon the uid and the pid. The client cannot change
// them. out carries the pty output to the writer of the client. The daemon
// closes done to stop that writer.
type client struct {
	uid  uint32
	pid  int32
	size size
	out  chan []byte
	done chan struct{}
}

// A window owns one pty and the clients that view it. The index of a window is
// its position in the windows slice.
type window struct {
	ptmx    *os.File
	clients map[net.Conn]client
}

// event is one line of the history log. The kind is attach, detach or switch.
type event struct {
	kind string
	uid  uint32
	pid  int32
	at   time.Time
}

// An event kind in the history log.
const (
	eventAttach = "attach"
	eventDetach = "detach"
	eventSwitch = "switch"
)

// historySize is the number of events that the daemon keeps. The oldest event
// leaves when the log is full.
const historySize = 1024

// A read of the pty does not stop. Therefore one goroutine reads each window
// for the life of the daemon and sends each chunk to the clients of that
// window. mu protects the windows, the view map, and the history. An empty
// window discards the output.
var (
	mu      sync.Mutex
	windows []*window
	view    = make(map[net.Conn]*window)
	events  []event
)

func Run() {
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to listen on socket:", err)
		os.Exit(1)
	}
	defer ln.Close()

	mu.Lock()
	if newWindowLocked() == nil {
		os.Exit(1)
	}
	mu.Unlock()

	fmt.Println("running roomux daemon on", socketPath)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to accept connection:", err)
			continue
		}
		cred, err := identity.Peer(conn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to verify the client:", err)
			conn.Close()
			continue
		}
		fmt.Printf("roomux: accepted connection from uid %d pid %d\n", cred.Uid, cred.Pid)

		go serve(conn, cred)
	}
}

// newWindowLocked spawns a pty and makes a window for it, then starts the
// reader of that window. The caller must hold mu. It returns nil when the spawn
// fails.
func newWindowLocked() *window {
	ptmx, err := pty.Spawn_pty()
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to spawn pty:", err)
		return nil
	}
	w := &window{ptmx: ptmx, clients: make(map[net.Conn]client)}
	windows = append(windows, w)
	go broadcast(w)
	return w
}

// ref names one client at one moment. broadcast keeps a copy of the clients of
// its window, thus it does not hold mu while it waits for a slow client.
type ref struct {
	conn net.Conn
	c    client
}

// broadcast reads the pty of one window and sends every chunk to the clients
// that view that window. Each client has its own writer goroutine and buffer,
// thus a client that does not read cannot stop the other clients: the daemon
// drops that client.
func broadcast(w *window) {
	buf := make([]byte, 4096)
	refs := make([]ref, 0, 8)
	for {
		n, err := w.ptmx.Read(buf)
		if n > 0 {
			// The next read overwrites buf, thus each chunk needs a copy of
			// its own. All the clients share the copy.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			refs = refs[:0]
			mu.Lock()
			for conn, c := range w.clients {
				refs = append(refs, ref{conn, c})
			}
			mu.Unlock()

			for _, r := range refs {
				deliver(r.conn, r.c, chunk)
			}
		}
		if err != nil {
			return
		}
	}
}

// deliver sends one chunk to one client. It waits for a client that is behind,
// but not longer than dropAfter: the daemon drops a client that does not read.
func deliver(conn net.Conn, c client, chunk []byte) {
	select {
	case c.out <- chunk:
		return
	case <-c.done:
		return
	default:
	}

	// The buffer is full. Give the writer of the client time to drain it. A
	// client that reads drains in milliseconds.
	timer := time.NewTimer(dropAfter)
	defer timer.Stop()
	select {
	case c.out <- chunk:
	case <-c.done:
	case <-timer.C:
		if drop(conn) {
			conn.Close()
		}
	}
}

// writeOut writes the pty output of one client. It stops when the daemon drops
// the client. A write error also stops it: the connection is then dead.
func writeOut(conn net.Conn, c client) {
	for {
		select {
		case chunk := <-c.out:
			if _, err := conn.Write(chunk); err != nil {
				drop(conn)
				conn.Close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// removeLocked deletes the client at conn from the window it views, stops its
// writer, and forgets the view. The delete and the close of done happen in one
// step, thus the writer of the client stops once. The caller must hold mu. It
// returns the client and false when the client is already gone.
func removeLocked(conn net.Conn) (client, bool) {
	w := view[conn]
	if w == nil {
		return client{}, false
	}
	c, ok := w.clients[conn]
	if !ok {
		return client{}, false
	}
	delete(w.clients, conn)
	delete(view, conn)
	close(c.done)
	return c, true
}

// drop removes one client from the set, stops its writer, and records the
// detach. It returns false when the client is already gone. A second call for
// the same conn does nothing.
func drop(conn net.Conn) bool {
	mu.Lock()
	c, ok := removeLocked(conn)
	if ok {
		recordLocked(eventDetach, c)
	}
	mu.Unlock()
	return ok
}

// recordLocked adds one event to the history. The oldest event leaves when the
// log is full. The caller must hold mu.
func recordLocked(kind string, c client) {
	if len(events) == historySize {
		copy(events, events[1:])
		events = events[:len(events)-1]
	}
	events = append(events, event{kind: kind, uid: c.uid, pid: c.pid, at: time.Now()})
}

// minSize gives the smallest rows and the smallest cols across the clients of
// one window, and zeros when no client sent a size. The caller must hold mu.
func minSize(w *window) (rows, cols uint16) {
	for _, c := range w.clients {
		if c.size.rows == 0 || c.size.cols == 0 {
			continue
		}
		if rows == 0 || c.size.rows < rows {
			rows = c.size.rows
		}
		if cols == 0 || c.size.cols < cols {
			cols = c.size.cols
		}
	}
	return rows, cols
}

// applySizeLocked gives the pty of one window the smallest size of its clients.
// A larger client then has an unused margin, but no client sees a broken
// screen. The caller must hold mu.
func applySizeLocked(w *window) {
	rows, cols := minSize(w)
	if rows == 0 || cols == 0 {
		return
	}
	if err := pty.Setsize(w.ptmx, rows, cols); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to set pty size:", err)
	}
}

// redrawLocked makes the programs in one window draw again after a client
// switches to that window. A size that does not change can leave the programs
// quiet, thus it sets a size that differs, then the true size. The caller must
// hold mu.
func redrawLocked(w *window) {
	rows, cols := minSize(w)
	if rows == 0 || cols == 0 {
		return
	}
	nudge := cols - 1
	if nudge == 0 {
		nudge = cols + 1
	}
	if err := pty.Setsize(w.ptmx, rows, nudge); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to set pty size:", err)
	}
	if err := pty.Setsize(w.ptmx, rows, cols); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to set pty size:", err)
	}
}

// writeInput sends the keystrokes of a client to the pty of the window that the
// client views. It returns false when the write fails.
func writeInput(conn net.Conn, payload []byte) bool {
	mu.Lock()
	w := view[conn]
	mu.Unlock()
	if w == nil {
		return false
	}
	if _, err := w.ptmx.Write(payload); err != nil {
		return false
	}
	return true
}

// setSizeLocked records the size of one client and sizes its window. The caller
// must hold mu.
func setSizeLocked(conn net.Conn, rows, cols uint16) {
	w := view[conn]
	if w == nil {
		return
	}
	c := w.clients[conn]
	c.size = size{rows: rows, cols: cols}
	w.clients[conn] = c
	applySizeLocked(w)
}

// switchLocked moves the client at conn to w, records the switch, and draws the
// new window again. A switch to the current window only draws again. The caller
// must hold mu.
func switchLocked(conn net.Conn, w *window) {
	old := view[conn]
	c, ok := old.clients[conn]
	if !ok {
		return
	}
	if old != w {
		delete(old.clients, conn)
		w.clients[conn] = c
		view[conn] = w
		recordLocked(eventSwitch, c)
		applySizeLocked(old)
	}
	redrawLocked(w)
}

// indexOf gives the index of one window, or -1. The caller must hold mu.
func indexOf(w *window) int {
	for i, x := range windows {
		if x == w {
			return i
		}
	}
	return -1
}

// cycleLocked moves the client at conn by delta windows, and wraps around. The
// caller must hold mu.
func cycleLocked(conn net.Conn, delta int) {
	if len(windows) == 0 {
		return
	}
	i := indexOf(view[conn])
	if i < 0 {
		return
	}
	n := len(windows)
	switchLocked(conn, windows[((i+delta)%n+n)%n])
}

// serve reads the first frame to find the purpose of the connection. A Who
// connection gets the list of the clients, and a History connection gets the
// history log; each then stops. An Attach connection joins the first window and
// reads frames until it stops.
func serve(conn net.Conn, cred *unix.Ucred) {
	defer conn.Close()

	kind, _, err := proto.Read(conn)
	if err != nil {
		return
	}

	switch kind {
	case proto.Who:
		if _, err := conn.Write([]byte(who())); err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to answer who:", err)
		}
		return
	case proto.History:
		if _, err := conn.Write([]byte(history())); err != nil {
			fmt.Fprintln(os.Stderr, "roomux: failed to answer history:", err)
		}
		return
	case proto.Attach:
	default:
		fmt.Fprintln(os.Stderr, "roomux: the first frame is not attach, who, or history")
		return
	}

	me := client{
		uid:  cred.Uid,
		pid:  cred.Pid,
		out:  make(chan []byte, outBuffer),
		done: make(chan struct{}),
	}
	mu.Lock()
	w := windows[0]
	w.clients[conn] = me
	view[conn] = w
	recordLocked(eventAttach, me)
	applySizeLocked(w)
	mu.Unlock()

	go writeOut(conn, me)

	// The label is necessary. A plain break in a switch does not stop the loop.
loop:
	for {
		kind, payload, err := proto.Read(conn)
		if err != nil {
			break
		}

		switch kind {
		case proto.Input:
			if !writeInput(conn, payload) {
				break loop
			}
		case proto.Resize:
			rows, cols, err := proto.ParseResize(payload)
			if err != nil {
				fmt.Fprintln(os.Stderr, "roomux:", err)
				continue
			}
			mu.Lock()
			setSizeLocked(conn, rows, cols)
			mu.Unlock()
		case proto.NewWindow:
			mu.Lock()
			if w := newWindowLocked(); w != nil {
				switchLocked(conn, w)
			}
			mu.Unlock()
		case proto.SelectWindow:
			i, err := proto.ParseIndex(payload)
			if err != nil {
				fmt.Fprintln(os.Stderr, "roomux:", err)
				continue
			}
			mu.Lock()
			if int(i) < len(windows) {
				switchLocked(conn, windows[i])
			}
			mu.Unlock()
		case proto.CycleWindow:
			delta, err := proto.ParseDelta(payload)
			if err != nil {
				fmt.Fprintln(os.Stderr, "roomux:", err)
				continue
			}
			mu.Lock()
			cycleLocked(conn, int(delta))
			mu.Unlock()
		}
	}

	mu.Lock()
	if c, ok := removeLocked(conn); ok {
		recordLocked(eventDetach, c)
	}
	mu.Unlock()

	fmt.Println("roomux: closed connection")
}

// who makes the text that lists the attached clients and their windows. It
// builds the text while it holds mu, and thus it does not write to a socket
// with mu held.
func who() string {
	type row struct {
		win int
		c   client
	}
	mu.Lock()
	rows := make([]row, 0, len(view))
	for i, w := range windows {
		for _, c := range w.clients {
			rows = append(rows, row{win: i, c: c})
		}
	}
	mu.Unlock()

	sort.Slice(rows, func(i, j int) bool { return rows[i].c.pid < rows[j].c.pid })

	var text strings.Builder
	fmt.Fprintf(&text, "%-12s %-6s %-8s %-8s %s\n", "USER", "UID", "PID", "WINDOW", "SIZE")
	for _, r := range rows {
		box := "-"
		if r.c.size.cols > 0 && r.c.size.rows > 0 {
			box = fmt.Sprintf("%dx%d", r.c.size.cols, r.c.size.rows)
		}
		fmt.Fprintf(&text, "%-12s %-6d %-8d %-8d %s\n", username(r.c.uid), r.c.uid, r.c.pid, r.win, box)
	}
	return text.String()
}

// history makes the text of the history log. The events are already in time
// order. It builds the text while it holds mu, and thus it does not write to a
// socket with mu held.
func history() string {
	mu.Lock()
	list := make([]event, len(events))
	copy(list, events)
	mu.Unlock()

	var text strings.Builder
	fmt.Fprintf(&text, "%-19s %-7s %-12s %-6s %s\n", "TIME", "EVENT", "USER", "UID", "PID")
	for _, e := range list {
		fmt.Fprintf(&text, "%-19s %-7s %-12s %-6d %d\n",
			e.at.Format("2006-01-02 15:04:05"), e.kind, username(e.uid), e.uid, e.pid)
	}
	return text.String()
}

// username finds the name of a uid. It gives the number if the lookup fails.
func username(uid uint32) string {
	id := strconv.FormatUint(uint64(uid), 10)
	u, err := user.LookupId(id)
	if err != nil {
		return id
	}
	return u.Username
}
