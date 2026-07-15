// Package identity verifies the real identity of each connecting client.
//
// read kernel peer credentials from the Unix domain
// socket (SO_PEERCRED via golang.org/x/sys/unix GetsockoptUcred) to establish
// each client's real UID, rather than trusting a self-reported name. This is
// the foundation for roomux's multi-user presence and history features.
package identity
