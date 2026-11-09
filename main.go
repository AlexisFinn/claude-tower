// Command claude-tower shows the Claude Code sessions running on this machine.
package main

import (
	"os"

	"claude-tower/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}
