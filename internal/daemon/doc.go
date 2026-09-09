// Package daemon implements the background server that owns sessions.
//
// The daemon listens on a Unix domain socket and owns the pty of the session.
// One permanent goroutine reads the pty and writes to every attached client,
// because a read of the pty does not stop. The daemon reads frames from each
// client: keystrokes go to the pty, and a size changes the pty size. The pty
// gets the smallest size of the attached clients.
package daemon
