// SPDX-License-Identifier: Apache-2.0

// version.go — Prints the build version, commit, and date.
//
//   - Version, Commit, and Date are populated via ldflags at build time
//     (see .goreleaser.yml and Makefile). When ldflags weren't set —
//     the common case for `go install` — the values fall back to
//     runtime/debug.ReadBuildInfo() so users still see a meaningful
//     module pseudo-version, VCS revision, and build time.
//   - The Version variable is also referenced by scan.go for invocation metadata.
package cmd

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Set via ldflags at build time. Defaults are sentinels meaning "use the
// runtime/debug.ReadBuildInfo fallback path."
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
			v, c, d := versionInfo()
			cmd.Printf("idio %s (commit: %s, built: %s)\n", v, c, d)
		},
	}
}

// versionInfo returns the effective version triple, preferring ldflags-injected
// values and falling back to module/VCS metadata embedded by the Go toolchain.
// Returns the original sentinels only if no build info is available either.
func versionInfo() (version, commit, date string) {
	version, commit, date = Version, Commit, Date
	if version != "dev" && commit != "none" && date != "unknown" {
		return // all set via ldflags
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return
	}
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if commit == "none" && s.Value != "" {
				commit = s.Value
				if len(commit) > 7 {
					commit = commit[:7]
				}
			}
		case "vcs.time":
			if date == "unknown" && s.Value != "" {
				date = s.Value
			}
		}
	}
	return
}
