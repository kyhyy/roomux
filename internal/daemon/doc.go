// Package daemon implements the background server that owns sessions.
//
// The daemon listens on a Unix domain socket and owns the pty of the session.
// One permanent goroutine reads the pty, because a read of the pty does not
// stop. It sends each chunk to a buffer that belongs to one client. One writer
// goroutine per client drains that buffer to the socket, thus a client that
// does not read cannot stop the other clients. The daemon waits for a client
// that is behind, then drops the client when the wait is too long. The daemon
// reads frames from each client: keystrokes go to the pty, and a size changes
// the pty size. The pty gets the smallest size of the attached clients. The
// daemon also keeps the history of the attach and detach events, with the
// verified uid and the time of each event.
package daemon
