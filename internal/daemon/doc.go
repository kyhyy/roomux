// Package daemon implements the background server that owns sessions.
//
// listen on a Unix domain socket, own the
// pty for each session, and stream its output to connected clients while
// forwarding their input. It also drives detach/reattach and, in later
// stages, fan-out to multiple simultaneous clients.
package daemon
