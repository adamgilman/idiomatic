// SPDX-License-Identifier: Apache-2.0

// cache.go — Manage idio's local clone cache.
//
//   - The cache holds shallow clones of capability/pack repositories
//     declared in .idiomatic.yaml. Once cloned, idio reuses the local
//     copy and never auto-pulls — so when an upstream adds a capability
//     the cached clone goes stale and `capability "X" is not loaded`
//     errors appear.
//   - `idio cache clear` removes the entire cache directory, forcing the
//     next scan to clone fresh.
//   - `idio cache path` prints the cache root for diagnostics.
package cmd

import (
	"fmt"
	"os"

	"github.com/adamgilman/idiomatic/internal/declarative"
	"github.com/spf13/cobra"
)

func NewCmdCache() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage idio's local clone cache",
		Long: "idiomatic clones capability and pack repos into a local cache " +
			"and never auto-pulls. Use these subcommands when an upstream " +
			"adds capabilities your cached clone doesn't have.",
	}
	cmd.AddCommand(newCmdCacheClear())
	cmd.AddCommand(newCmdCachePath())
	return cmd
}

func newCmdCacheClear() *cobra.Command {
	return &cobra.Command{
		Use:   "clear",
		Short: "Remove all cached repository clones",
		Long: "Removes the entire cache directory. The next scan will re-clone " +
			"every capability/pack source declared in .idiomatic.yaml.",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := declarative.CacheDir()
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				cmd.Printf("cache is already empty: %s\n", dir)
				return nil
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("remove %s: %w", dir, err)
			}
			cmd.Printf("cleared cache: %s\n", dir)
			return nil
		},
	}
}

func newCmdCachePath() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the cache directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println(declarative.CacheDir())
			return nil
		},
	}
}
