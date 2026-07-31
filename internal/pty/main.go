package pty

import (
	"io"
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

func Attach(ptmx *os.File, conn io.ReadWriter) {

	go func() {
		_, _ = io.Copy(conn, ptmx)
	}()
	_, _ = io.Copy(ptmx, conn)
}
