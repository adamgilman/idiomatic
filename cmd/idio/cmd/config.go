// SPDX-License-Identifier: Apache-2.0

// config.go — Hidden placeholder for future CLI configuration management.
//
//   - All subcommands (view, set) are Hidden from --help and are no-ops.
//   - No actual configuration is persisted yet; "set" prints but discards
//     the value. This file exists to reserve the command namespace.
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewCmdConfig() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "config",
		Short:  "Manage idio configuration",
		Long:   "View and modify the idio CLI configuration.",
		Hidden: true,
	}

	cmd.AddCommand(newCmdConfigView())
	cmd.AddCommand(newCmdConfigSet())

	return cmd
}

func newCmdConfigView() *cobra.Command {
	return &cobra.Command{
		Use:   "view",
		Short: "View current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("No configuration set.")
			return nil
		},
	}
}

func newCmdConfigSet() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Set %s = %s\n", args[0], args[1])
			return nil
		},
	}
}
