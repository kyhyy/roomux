// Package client implements the client side of roomux.
//
// The client connects to the socket of the daemon and puts the local terminal
// into raw mode. It sends the keystrokes and the terminal size as frames, and
// it shows the raw pty output that comes back. Ctrl+Space opens the command
// bar: there the user makes a window, selects a window, moves to the next or
// the previous window, or detaches. The client sends every other key to the
// shell. Who and History use a second connection to ask the daemon which
// clients are present, or for the history log.
package client
