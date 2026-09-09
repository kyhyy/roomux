// Package proto defines the frames that a client sends to the daemon.
//
// The client sends keystrokes and terminal sizes on one connection, thus each
// message needs a kind. The daemon sends raw pty output in the other
// direction, because it has only one kind of message.
package proto
