// Package daemon implements the background server that owns sessions.
//
// The daemon listens on a Unix domain socket and owns the pty of the session.
// One permanent goroutine reads the pty and writes to the attached client,
// because a read of the pty does not stop. The daemon reads frames from the
// client: keystrokes go to the pty, and a size changes the pty size. Only one
// client attaches at a time. A later stage makes the slot a set of clients.
package daemon
