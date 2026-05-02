// SPDX-License-Identifier: Apache-2.0

// root.go — Assembles the top-level cobra command tree for idio.
//
//   - SilenceUsage and SilenceErrors are set so the CLI controls its own
//     error output rather than letting cobra print usage on every error.
//   - Every subcommand (scan, manifest, version) is registered
//     here via AddCommand; add new top-level commands in this function.
package cmd

import (
	"github.com/spf13/cobra"
)

func NewCmdRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "idio",
		Short: "Idiomatic CLI",
		Long:  "Deterministic enforcement for stochastic development.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(NewCmdVersion())
	cmd.AddCommand(NewCmdManifest())
	cmd.AddCommand(NewCmdScan())

	return cmd
}
