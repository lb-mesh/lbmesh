package main

import (
	"fmt"
	"os"

	"github.com/lbmesh/lbmesh-v2/internal/cli"
)

// printTrailingSpace leaves blank lines after all output so the shell prompt
// does not touch the last line. It is skipped when stdout is not a terminal.
func printTrailingSpace() {
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		fmt.Print("\n\n")
	}
}

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		printTrailingSpace()
		os.Exit(1)
	}
	printTrailingSpace()
}
