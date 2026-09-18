// Command staredit edits StarRupture dedicated-server saves.
package main

import (
	"fmt"
	"os"

	"github.com/tulpenhaendler/StarEdit/internal/cli"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "staredit:", err)
		os.Exit(1)
	}
}
