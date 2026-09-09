// Package daemon implements the background server that owns sessions.
//
// It listens on a Unix domain socket and owns the pty for each session. A
// single permanent goroutine reads the pty master and writes to whichever
// client is attached, because a goroutine blocked in that read cannot be
// interrupted without closing the pty and killing the shell. One client is
// attached at a time; a new attach evicts the previous one. Later stages turn
// that single slot into a set, for fan-out to simultaneous clients.
package daemon
