// Package daemon implements the background server that owns the session.
//
// The daemon listens on a Unix domain socket and owns the windows. A window is
// one pty with a shell and the clients that view it. One permanent goroutine
// reads each pty, because a read of the pty does not stop. It sends each chunk as
// an Output frame to a buffer that belongs to one client. One writer goroutine
// per client drains that buffer to the socket, thus a client that does not read
// cannot stop the other clients. The daemon waits for a client that is behind,
// then drops the client when the wait is too long.
//
// The daemon reads frames from each client: keystrokes go to the pty of the
// window that the client views, a size changes the size of that pty, and a window
// frame makes a window, selects a window, or moves to the next or the previous
// window. It sends a Status frame to a client after each change that touches it,
// with the window list and the number of clients in its window. Each pty gets the
// smallest size of the clients that view it. The daemon also keeps the history of
// the attach, detach, and switch events, with the verified uid and the time of
// each event.
package daemon
