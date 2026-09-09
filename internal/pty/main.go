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
