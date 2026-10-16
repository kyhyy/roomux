// Package proto defines the frames that a client sends to the daemon.
//
// The client sends keystrokes, terminal sizes, and window commands on one
// connection, thus each message needs a kind. The first frame tells the daemon
// if the connection attaches to a window, asks who is present, or asks for the
// history. The daemon does not use frames, because it sends only one type of
// data on each connection: pty output to a client that attaches, or text to a
// client that asks who is present or asks for the history.
package proto
