// Package client implements the client side of roomux.
//
// The client connects to the socket of the daemon and puts the local terminal
// into raw mode. It sends the keystrokes and the terminal size as frames, and it
// reads the Output frames that come back and writes them to the terminal.
// Ctrl+Space opens the command bar: there the user makes a window, selects a
// window, moves to the next or the previous window, or detaches. The client
// sends every other key to the shell. It keeps the bottom row of the terminal
// for a Status frame and for the command bar, and it tells the daemon that the
// programs have one row less. Who and History use a second connection to ask the
// daemon which clients are present, or for the history log.
package client
