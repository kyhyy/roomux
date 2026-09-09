// Package proto defines the frames that a client sends to the daemon.
//
// The client sends keystrokes and terminal sizes on one connection, thus each
// message needs a kind. The first frame tells the daemon if the connection
// attaches to the shell or asks who is present. The daemon does not use
// frames, because it sends only one type of data on each connection: pty
// output to a client that attaches, or text to a client that asks who is
// present.
package proto
