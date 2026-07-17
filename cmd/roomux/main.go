package main

import (
	"fmt"
	"os"
	"roomux/internal/client"
	"roomux/internal/daemon"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "attach":
		client.Connect()
	case "daemon":
		daemon.Run()
	case "help":
		fmt.Println("usage: roomux <command> [args]")
		fmt.Println()
		fmt.Println("commands:")
		fmt.Println("  help")
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
	fmt.Println("  help")
	fmt.Println("  attach")
	fmt.Println("  daemon")
}
