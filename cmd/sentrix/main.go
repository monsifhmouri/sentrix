// Command sentrix is the entry point for SENTRIX, a passive attack-surface
// intelligence tool by MONSIF HMOURI (MONSTR MIND LABS).
package main

import (
	"os"

	"sentrix/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
