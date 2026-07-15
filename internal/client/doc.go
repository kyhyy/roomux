// Package client implements the client side of roomux.
//
// connect to the daemon's Unix domain socket,
// put the local terminal into raw mode, and stream keystrokes to the daemon
// while rendering the pty output it sends back. Supports detach/reattach.
package client
