// Package pty handles spawning a shell inside a pseudo-terminal and moving
// bytes between that pty and the rest of roomux.
//
// use creack/pty to start a shell, then pipe local
// stdin/stdout through the pty master in a single process (no daemon yet).
// Later stages hand pty ownership to the daemon.
package pty
