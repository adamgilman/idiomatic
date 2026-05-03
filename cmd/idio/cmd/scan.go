// SPDX-License-Identifier: Apache-2.0

// scan.go — The primary scan command that orchestrates rule evaluation.
//
//   - Supports three output formats: human (default), sarif, and claude-hook.
//   - claude-hook mode reads stdin for the Claude hook payload and always
//     exits 0 (errors go in additionalContext per the hook contract).
//   - Version constraints on rules are validated against loaded capabilities
//     before any tool execution, so stale pins fail fast.
//   - Exit code 1 means findings were reported; exit code 2 means a setup
//     or configuration error occurred.
//   - File collection is intentionally broad (all files under a directory);
//     each capability further filters by its own AppliesToFiles.Extensions.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/adamgilman/idiomatic/config"
	"github.com/adamgilman/idiomatic/internal/claudehook"
	"github.com/adamgilman/idiomatic/internal/declarative"
	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/internal/output"
	manifestlib "github.com/adamgilman/idiomatic/manifest"
	"github.com/spf13/cobra"
)

func NewCmdScan() *cobra.Command {
	var configPath string
	var formatFlag string

	cmd := &cobra.Command{
		Use:   "scan <path> [paths...]",
		Short: "Scan files against loaded rules",
		Long:  "Load a rule manifest, scan the given files or directories, and report findings.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine working directory: %w", err)
			}

			meta := output.InvocationMetadata{
				ToolName:    "idio",
				ToolVersion: Version,
				WorkingDir:  cwd,
			}

			if formatFlag == "claude-hook" {
				return runClaudeHook(ctx, cmd, configPath, meta)
			}

			var formatter output.Formatter
			switch formatFlag {
			case "human", "":
				formatter = &output.HumanFormatter{}
			case "sarif":
				formatter = &output.SARIFFormatter{}
			default:
				fmt.Fprintf(os.Stderr, "error: unknown format %q (use 'human', 'sarif', or 'claude-hook')\n", formatFlag)
				os.Exit(2)
			}

			if len(args) == 0 {
				fmt.Fprintf(os.Stderr, "error: at least one path argument is required\n")
				os.Exit(2)
			}

			// Discover config if not provided.
			cfgPath := configPath
			if cfgPath == "" {
				cfgPath = config.Find(cwd)
			}
			if cfgPath == "" {
				return emitErrorAndExit(formatter, nil, nil, meta,
					"no .idiomatic.yaml found: pass --config <path> or run from a project containing .idiomatic.yaml", formatFlag)
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				return emitErrorAndExit(formatter, nil, nil, meta, err.Error(), formatFlag)
			}

			project, err := config.LoadProject(cfg)
			if err != nil {
				return emitErrorAndExit(formatter, nil, nil, meta, err.Error(), formatFlag)
			}

			rules := project.Rules
			registry := project.Registry

			var files []string
			for _, path := range args {
				collected, err := collectFiles(path)
				if err != nil {
					return emitErrorAndExit(formatter, rules, files, meta, err.Error(), formatFlag)
				}
				files = append(files, collected...)
			}

			// Validate each rule's version constraint against the loaded
			// capability before any tool runs. A stale rule pinned to an
			// older capability major version fails loudly here instead of
			// silently producing wrong findings.
			versionRequests := make([]declarative.RuleVersionRequest, 0, len(rules))
			for _, rule := range rules {
				versionRequests = append(versionRequests, declarative.RuleVersionRequest{
					RuleID:     rule.ID,
					Capability: rule.Detector.Capability,
					Constraint: rule.Detector.Version,
				})
			}
			if err := registry.CheckRuleVersions(versionRequests); err != nil {
				return emitErrorAndExit(formatter, rules, files, meta, err.Error(), formatFlag)
			}

			backends := registry.BuildBackends()

			rulesByCapability := make(map[string][]manifestlib.Rule)
			for _, rule := range rules {
				rulesByCapability[rule.Detector.Capability] = append(rulesByCapability[rule.Detector.Capability], rule)
			}

			var allFindings []engine.Finding
			for capName, capRules := range rulesByCapability {
				backend, ok := backends[capName]
				if !ok {
					if _, known := registry.ByName(capName); !known {
						return emitErrorAndExit(formatter, rules, files, meta,
							fmt.Sprintf("capability %q is not loaded by any capability source\n"+
								"  hint: if your .idiomatic.yaml lists this capability, your local clone may be stale.\n"+
								"        try: idio cache clear", capName), formatFlag)
					}
					return emitErrorAndExit(formatter, rules, files, meta,
						fmt.Sprintf("capability %q binary is not installed", capName), formatFlag)
				}

				findings, err := backend.Analyze(ctx, files, capRules)
				if err != nil {
					return emitErrorAndExit(formatter, rules, files, meta, err.Error(), formatFlag)
				}
				allFindings = append(allFindings, findings...)
			}

			out, err := formatter.Format(allFindings, rules, files, meta)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(2)
			}

			if len(out) > 0 {
				_, _ = cmd.OutOrStdout().Write(out)
			}

			if len(allFindings) > 0 {
				os.Exit(1)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "Path to .idiomatic.yaml (default: walk up from CWD)")
	cmd.Flags().StringVarP(&formatFlag, "format", "f", "human", "Output format: human, sarif, claude-hook")

	return cmd
}

// runClaudeHook handles --format=claude-hook. Missing binaries are
// reported as error envelopes in additionalContext (exit 0, per hook contract).
func runClaudeHook(ctx context.Context, cmd *cobra.Command, configPath string, meta output.InvocationMetadata) error {
	stdinData, err := io.ReadAll(os.Stdin)
	if err != nil {
		stdinData = []byte("{}")
	}

	// Discover config if not provided. Walk up from the EDITED FILE's directory
	// (extracted from the hook payload) rather than idio's cwd — the hook is
	// typically invoked from Claude Code's session cwd which is rarely the
	// same as the project being edited.
	cfgPath := configPath
	if cfgPath == "" {
		if dir := hookEditDir(stdinData); dir != "" {
			cfgPath = config.Find(dir)
		}
		if cfgPath == "" {
			cfgPath = config.Find(meta.WorkingDir)
		}
	}

	var registry *declarative.Registry
	var rules []manifestlib.Rule

	if cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err == nil {
			project, err := config.LoadProject(cfg)
			if err == nil {
				registry = project.Registry
				rules = project.Rules
			}
		}
	}

	// Fall back to empty registry if config loading failed.
	if registry == nil {
		registry = declarative.NewRegistry()
	}

	backends := registry.BuildBackends()

	resp, _ := claudehook.Run(ctx, stdinData, backends, registry, rules, meta)
	if len(resp) > 0 {
		_, _ = cmd.OutOrStdout().Write(resp)
	}
	return nil
}

// hookEditDir extracts the parent directory of the file referenced in a
// PostToolUse hook payload. Returns "" when the payload doesn't contain a
// usable file_path. Used by claude-hook mode to discover the project's
// .idiomatic.yaml from the edited file's location, not idio's own cwd.
func hookEditDir(stdinData []byte) string {
	var input claudehook.HookInput
	if err := json.Unmarshal(stdinData, &input); err != nil {
		return ""
	}
	var ti claudehook.EditToolInput
	if err := json.Unmarshal(input.ToolInput, &ti); err != nil {
		return ""
	}
	if ti.FilePath == "" {
		return ""
	}
	abs, err := filepath.Abs(ti.FilePath)
	if err != nil {
		return ""
	}
	return filepath.Dir(abs)
}

func emitErrorAndExit(formatter output.Formatter, rules []manifestlib.Rule, files []string, meta output.InvocationMetadata, errMsg, format string) error {
	if format == "sarif" {
		meta.ExecError = errMsg
		out, _ := formatter.Format(nil, rules, files, meta)
		if len(out) > 0 {
			_, _ = os.Stdout.Write(out)
		}
	} else {
		fmt.Fprintf(os.Stderr, "error: %s\n", errMsg)
	}
	os.Exit(2)
	return nil
}

// collectFiles returns the file list for one CLI argument. A regular file is
// returned as-is; a directory is walked recursively. Each capability filters
// the file set further via its own AppliesToFiles.Extensions, so we don't
// need a top-level extension filter here.
func collectFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", path, err)
	}
	return files, nil
}
