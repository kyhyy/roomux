// Package proto defines the frames that roomux sends over one connection.
//
// The client sends keystrokes, terminal sizes, and window commands on an attach
// connection, thus each message needs a kind. The daemon sends Output frames
// (pty bytes) and Status frames (the status line) back on that same connection.
// The first frame tells the daemon if the connection attaches to a window, asks
// who is present, or asks for the history. A Who or a History connection gets
// raw text and no frames, because it carries one type of data.
package proto
