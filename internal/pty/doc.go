// Package pty handles spawning a shell inside a pseudo-terminal.
//
// Spawn_pty uses creack/pty to start a shell and returns the pty master. The
// daemon owns that master from then on and is the only thing that reads it;
// this package does not move bytes itself.
package pty
