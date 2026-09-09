// Package client implements the client side of roomux.
//
// It connects to the daemon's Unix domain socket, puts the local terminal into
// raw mode, and streams keystrokes to the daemon while rendering the pty output
// it sends back. Ctrl+B d detaches; every other key is forwarded untouched, so
// the shell still sees Ctrl+C, Ctrl+D and friends.
package client
