// SPDX-License-Identifier: Apache-2.0

// discover.go — Runtime binary and file discovery, plus preflight prechecks.
//
//   - resolveRuntime produces a runtimeCtx (binary path, project root,
//     discovered files) that is threaded through argv expansion, config-file
//     rendering, and template evaluation for the rest of the pipeline.
//   - walk_up_from_cwd discovery traverses parent directories to find
//     locally-installed binaries (e.g. node_modules/.bin/eslint); the
//     directory containing the match becomes {project_root}.
//   - Prechecks run per unique input value (for_each_unique_input) and abort
//     before tool invocation with a templated error, catching missing plugins
//     or dependencies early.
package declarative

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/adamgilman/idiomatic/manifest"
)

// runtimeCtx holds everything resolved before invocation: the binary path,
// the project root (from binary discovery), and the named files discovered
// via DiscoverFiles. It is the input to argv expansion, config_file path
// resolution, and template rendering.
type runtimeCtx struct {
	BinaryPath  string
	ProjectRoot string
	Discovered  map[string]string
}

// resolveRuntime walks the spec's discovery declarations and produces the
// runtimeCtx for an Analyze call. Aborts with a clear error if a required
// discovery fails.
func (c *Capability) resolveRuntime(req engineFiles) (*runtimeCtx, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}
	rt := &runtimeCtx{
		ProjectRoot: cwd,
		Discovered:  map[string]string{},
	}

	// 1. Binary discovery
	bin, projectRoot, err := c.discoverBinary(cwd)
	if err != nil {
		return nil, err
	}
	rt.BinaryPath = bin
	if projectRoot != "" {
		rt.ProjectRoot = projectRoot
	}

	// 2. File discovery (uses the resolved project root if available, else CWD)
	for _, df := range c.spec.Spec.Requires.DiscoverFiles {
		start := cwd
		if df.WalkUpFrom == "" || df.WalkUpFrom == "cwd" {
			start = cwd
		}
		path := walkUpForFile(start, df.Name)
		if path == "" && !df.Optional {
			return nil, fmt.Errorf("capability %q: required file %q not found walking up from %s",
				c.spec.Metadata.Name, df.Name, start)
		}
		rt.Discovered[df.Var] = path
	}

	return rt, nil
}

// discoverBinary returns (binaryPath, projectRoot). projectRoot is non-empty
// when discovery used walk_up; empty when the binary was found via PATH.
func (c *Capability) discoverBinary(cwd string) (string, string, error) {
	binName := c.spec.Spec.Requires.Binary
	d := c.spec.Spec.Requires.Discover

	if d == nil {
		// Default behavior: exec.LookPath only.
		path, err := exec.LookPath(binName)
		if err == nil {
			return path, "", nil
		}
		return "", "", c.notFoundError()
	}

	switch d.Strategy {
	case "walk_up_from_cwd":
		dir := cwd
		for {
			candidate := filepath.Join(dir, d.Relative)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		if d.FallbackToPath {
			if path, err := exec.LookPath(binName); err == nil {
				return path, "", nil
			}
		}
		return "", "", c.notFoundError()
	default:
		return "", "", fmt.Errorf("capability %q: unknown discover strategy %q", c.spec.Metadata.Name, d.Strategy)
	}
}

// walkUpForFile walks parent directories from start looking for a file
// named name. Returns the first match's absolute path, or "" if none found.
func walkUpForFile(start, name string) string {
	dir := start
	for {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// notFoundError builds a consistent "binary not found" error using the
// spec's install hint.
func (c *Capability) notFoundError() error {
	if hint := c.spec.Spec.Requires.Install; hint != "" {
		return fmt.Errorf("%s not found; install with: %s", c.spec.Spec.Requires.Binary, hint)
	}
	return fmt.Errorf("%s not found", c.spec.Spec.Requires.Binary)
}

// runPrechecks evaluates each precheck spec against the rules being analyzed.
// The first failing check aborts with a templated error message.
func (c *Capability) runPrechecks(rt *runtimeCtx, rules []manifest.Rule) error {
	for _, pc := range c.spec.Spec.Requires.Precheck {
		if pc.Kind != "file_exists" {
			return fmt.Errorf("capability %q: unknown precheck kind %q", c.spec.Metadata.Name, pc.Kind)
		}

		values := uniquePrecheckValues(pc, rules)
		if len(values) == 0 && pc.ForEachUniqueInput == "" {
			values = []string{""} // run once with no value
		}

		for _, v := range values {
			data := struct {
				ProjectRoot string
				Discovered  map[string]string
				Value       string
			}{
				ProjectRoot: rt.ProjectRoot,
				Discovered:  rt.Discovered,
				Value:       v,
			}
			path, err := renderPrecheckTemplate(pc.Path, data)
			if err != nil {
				return fmt.Errorf("capability %q: render precheck path: %w", c.spec.Metadata.Name, err)
			}
			path = strings.ReplaceAll(path, "{project_root}", rt.ProjectRoot)
			if _, err := os.Stat(path); err == nil {
				continue
			}
			msg, err := renderPrecheckTemplate(pc.ErrorMessage, data)
			if err != nil {
				return fmt.Errorf("capability %q: render precheck error: %w", c.spec.Metadata.Name, err)
			}
			return fmt.Errorf("%s", strings.ReplaceAll(msg, "{project_root}", rt.ProjectRoot))
		}
	}
	return nil
}

// uniquePrecheckValues collects the unique non-empty, non-skipped values of
// the named input field across the rule set. Used to drive
// for_each_unique_input prechecks.
func uniquePrecheckValues(pc PrecheckSpec, rules []manifest.Rule) []string {
	if pc.ForEachUniqueInput == "" {
		return nil
	}
	skip := map[string]bool{}
	for _, s := range pc.SkipValues {
		skip[s] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range rules {
		v, _ := r.Detector.Config[pc.ForEachUniqueInput].(string)
		if v == "" || skip[v] || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func renderPrecheckTemplate(s string, data any) (string, error) {
	if !strings.Contains(s, "{{") {
		return s, nil
	}
	t, err := template.New("precheck").Parse(s)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// engineFiles is a tiny indirection so resolveRuntime doesn't have to import
// engine.AnalysisRequest. (It currently doesn't need the file list, but
// keeping the parameter makes the signature stable for future use.)
type engineFiles struct {
	Files []string
}
