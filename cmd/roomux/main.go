package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	default:
		fmt.Fprintf(os.Stderr, "roomux: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("usage: roomux <command> [args]")
	fmt.Println()
	fmt.Println("commands:")
	fmt.Println("  (none yet)")
}
