// SPDX-License-Identifier: Apache-2.0

// main.go — CLI entry point for the idio binary.
//
//   - Delegates entirely to the cobra root command; all logic lives in cmd/.
//   - Errors are printed to stderr and the process exits non-zero.
package main

import (
	"fmt"
	"os"

	"github.com/adamgilman/idiomatic/cmd/idio/cmd"
)

func main() {
	if err := cmd.NewCmdRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
