// Package client implements the client side of roomux.
//
// The client connects to the socket of the daemon and puts the local terminal
// into raw mode. It sends the keystrokes and the terminal size as frames, and
// it shows the raw pty output that comes back. Ctrl+B d detaches.
package client
