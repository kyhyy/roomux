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

// event is one line of the history log. The kind is attach or detach.
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
)

// historySize is the number of events that the daemon keeps. The oldest event
// leaves when the log is full.
const historySize = 1024

// A read of the pty does not stop. Therefore one goroutine reads the pty for
// the life of the daemon and sends each chunk to the clients. mu protects the
// set and the history. An empty set means that the daemon discards the output.
var (
	mu      sync.Mutex
	clients = make(map[net.Conn]client)
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

	ptmx, err := pty.Spawn_pty()
	if err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to spawn pty:", err)
		os.Exit(1)
	}
	defer func() {
		_ = ptmx.Close()
	}()

	go broadcast(ptmx)

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

		go serve(ptmx, conn, cred)
	}
}

// ref names one client at one moment. broadcast keeps a copy of the set, thus
// it does not hold mu while it waits for a slow client.
type ref struct {
	conn net.Conn
	c    client
}

// broadcast reads the pty and sends every chunk to every attached client. Each
// client has its own writer goroutine and buffer, thus a client that does not
// read cannot stop the other clients: the daemon drops that client.
func broadcast(ptmx *os.File) {
	buf := make([]byte, 4096)
	refs := make([]ref, 0, 8)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			// The next read overwrites buf, thus each chunk needs a copy of
			// its own. All the clients share the copy.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			refs = refs[:0]
			mu.Lock()
			for conn, c := range clients {
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

// removeLocked deletes one client and stops its writer. The delete and the
// close of done happen in one step, thus the writer of the client stops once.
// The caller must hold mu. It returns the client and false when the client is
// already gone.
func removeLocked(conn net.Conn) (client, bool) {
	c, ok := clients[conn]
	if !ok {
		return client{}, false
	}
	delete(clients, conn)
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

// applySize gives the pty the smallest size of the attached clients. A larger
// client then has an unused margin, but no client sees a broken screen. The
// caller must hold mu. A client that sent no size has a zero size, thus
// applySize ignores it.
func applySize(ptmx *os.File) {
	var min size
	for _, c := range clients {
		if c.size.rows == 0 || c.size.cols == 0 {
			continue
		}
		if min.rows == 0 || c.size.rows < min.rows {
			min.rows = c.size.rows
		}
		if min.cols == 0 || c.size.cols < min.cols {
			min.cols = c.size.cols
		}
	}
	if min.rows == 0 || min.cols == 0 {
		return
	}
	if err := pty.Setsize(ptmx, min.rows, min.cols); err != nil {
		fmt.Fprintln(os.Stderr, "roomux: failed to set pty size:", err)
	}
}

// serve reads the first frame to find the purpose of the connection. A Who
// connection gets the list of the clients, and a History connection gets the
// history log; each then stops. An Attach connection joins the set of clients
// and gets the pty output.
func serve(ptmx *os.File, conn net.Conn, cred *unix.Ucred) {
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
	clients[conn] = me
	recordLocked(eventAttach, me)
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
			if _, err := ptmx.Write(payload); err != nil {
				break loop
			}
		case proto.Resize:
			rows, cols, err := proto.ParseResize(payload)
			if err != nil {
				fmt.Fprintln(os.Stderr, "roomux:", err)
				continue
			}
			mu.Lock()
			c := clients[conn]
			c.size = size{rows: rows, cols: cols}
			clients[conn] = c
			applySize(ptmx)
			mu.Unlock()
		}
	}

	mu.Lock()
	if c, ok := removeLocked(conn); ok {
		recordLocked(eventDetach, c)
	}
	applySize(ptmx)
	mu.Unlock()

	fmt.Println("roomux: closed connection")
}

// who makes the text that lists the attached clients. It builds the text while
// it holds mu, and thus it does not write to a socket with mu held.
func who() string {
	mu.Lock()
	list := make([]client, 0, len(clients))
	for _, c := range clients {
		list = append(list, c)
	}
	mu.Unlock()

	sort.Slice(list, func(i, j int) bool { return list[i].pid < list[j].pid })

	var text strings.Builder
	fmt.Fprintf(&text, "%-12s %-6s %-8s %s\n", "USER", "UID", "PID", "SIZE")
	for _, c := range list {
		box := "-"
		if c.size.cols > 0 && c.size.rows > 0 {
			box = fmt.Sprintf("%dx%d", c.size.cols, c.size.rows)
		}
		fmt.Fprintf(&text, "%-12s %-6d %-8d %s\n", username(c.uid), c.uid, c.pid, box)
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
