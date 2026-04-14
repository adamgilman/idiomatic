// SPDX-License-Identifier: Apache-2.0

// version.go — Prints the build version, commit, and date.
//
//   - Version, Commit, and Date are populated via ldflags at build time
//     (see .goreleaser.yml and Makefile); they default to "dev"/"none"/"unknown".
//   - The Version variable is also referenced by scan.go for invocation metadata.
package cmd

import (
	"github.com/spf13/cobra"
)

// Set via ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func NewCmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of idio",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("idio %s (commit: %s, built: %s)\n", Version, Commit, Date)
		},
	}
}
