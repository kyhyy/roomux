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

	"golang.org/x/sys/unix"
)

const socketPath = "/tmp/roomux.sock"

type size struct {
	rows uint16
	cols uint16
}

// The kernel gives the daemon the uid and the pid. The client cannot change
// them.
type client struct {
	uid  uint32
	pid  int32
	size size
}

// A read of the pty does not stop. Therefore one goroutine reads the pty for
// the life of the daemon and writes to every attached client. mu protects the
// set. An empty set means that the daemon discards the output.
var (
	mu      sync.Mutex
	clients = make(map[net.Conn]client)
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

// broadcast reads the pty and writes to every attached client.
func broadcast(ptmx *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if n > 0 {
			mu.Lock()
			for conn := range clients {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					delete(clients, conn)
				}
			}
			mu.Unlock()
		}
		if err != nil {
			return
		}
	}
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
// connection gets the list of the clients and then stops. An Attach connection
// joins the set of clients and gets the pty output.
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
	case proto.Attach:
	default:
		fmt.Fprintln(os.Stderr, "roomux: the first frame is not attach or who")
		return
	}

	mu.Lock()
	clients[conn] = client{uid: cred.Uid, pid: cred.Pid}
	mu.Unlock()

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
	delete(clients, conn)
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

// username finds the name of a uid. It gives the number if the lookup fails.
func username(uid uint32) string {
	id := strconv.FormatUint(uint64(uid), 10)
	u, err := user.LookupId(id)
	if err != nil {
		return id
	}
	return u.Username
}
