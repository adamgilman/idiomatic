// SPDX-License-Identifier: Apache-2.0

// manifest.go — Subcommands for validating and listing rule pack manifests.
//
//   - "validate" loads manifests and reports all warnings/errors at once
//     rather than failing on the first error.
//   - "list" requires a clean manifest (no validation errors) before
//     displaying rules; this prevents misleading partial output.
//   - Both subcommands accept a path that can be a single YAML file or
//     a directory of pack files.
package cmd

import (
	"fmt"
	"os"

	manifestlib "github.com/adamgilman/idiomatic/manifest"
	"github.com/spf13/cobra"
)

func NewCmdManifest() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Manage rule manifests",
		Long:  "Load, validate, and inspect rule manifest files.",
	}

	cmd.AddCommand(newCmdValidate())
	cmd.AddCommand(newCmdList())

	return cmd
}

func newCmdValidate() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <path>",
		Short: "Validate a manifest file or directory",
		Long:  "Parse and validate rule manifests, reporting all errors at once.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			result, err := manifestlib.Load(path)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}

			// Print warnings.
			for _, w := range result.Warnings {
				cmd.PrintErrln("warning:", w)
			}

			// Print errors.
			if len(result.Errors) > 0 {
				cmd.PrintErrln()
				for _, e := range result.Errors {
					cmd.PrintErrln("error:", e.Error())
				}
				cmd.PrintErrln()
				cmd.PrintErrf("Validation failed with %d error(s).\n", len(result.Errors))
				os.Exit(1)
			}

			cmd.Println(result.Summary())
			cmd.Println("Validation passed.")
			return nil
		},
	}
}

func newCmdList() *cobra.Command {
	return &cobra.Command{
		Use:   "list <path>",
		Short: "List rules in a manifest file or directory",
		Long:  "Load manifests and display a summary of all rules.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			result, err := manifestlib.Load(path)
			if err != nil {
				return fmt.Errorf("failed to load manifest: %w", err)
			}

			if len(result.Errors) > 0 {
				for _, e := range result.Errors {
					cmd.PrintErrln("error:", e.Error())
				}
				return fmt.Errorf("manifest has validation errors; fix them first")
			}

			cmd.Println(result.Summary())
			cmd.Println()

			for _, m := range result.Manifests {
				cmd.Printf("Pack: %s (%s) v%s\n", m.Pack.Name, m.Pack.ID, m.Pack.Version)
				cmd.Printf("  %s\n", m.Pack.Description)
				cmd.Println()

				for _, r := range m.Rules {
					tested := "untested"
					if len(r.Tests) > 0 {
						tested = fmt.Sprintf("%d test(s)", len(r.Tests))
					}
					cmd.Printf("  %-40s  %-8s  %s\n", r.ID, r.Severity, tested)
					cmd.Printf("    %s\n", r.Name)
				}
			}

			return nil
		},
	}
}
