package pty

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

func Spawn_pty() (*os.File, error) {

	c := exec.Command("bash")

	ptmx, err := pty.Start(c)
	if err != nil {
		return nil, err
	}
	return ptmx, nil
}

// Setsize changes the size of the pty. The kernel then sends SIGWINCH to the
// programs in the pty, and they redraw the screen.
func Setsize(ptmx *os.File, rows, cols uint16) error {
	return pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}
