// SPDX-License-Identifier: Apache-2.0

// run.go — Tool execution: process spawning, argv/template expansion, config
// file rendering, temp file lifecycle, and output capture.
//
//   - Three invocation modes: runBatch (one call, all rules), runPerFile (one
//     call per target file), runPerRule (one call per rule). The mode is
//     selected by spec.run.per and signal.shape.
//   - Argv entries can be literal tokens ({files}, {config_file}, {report_path}),
//     Go templates (rendered via sprig + toYaml), or plain strings. The custom
//     toYaml template func is added because sprig ships toJson but not toYaml.
//   - Config files are written to temp paths or directly into {project_root};
//     both are cleaned up via deferred os.Remove in the calling function.
//   - Exit code 0 is always OK; additional accepted codes are declared in
//     spec.run.ok_exit (e.g. [1] for tools that exit 1 on findings).
package declarative

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/adamgilman/idiomatic/internal/engine"
	"github.com/adamgilman/idiomatic/manifest"
	"gopkg.in/yaml.v3"
)

// templateFuncs returns the sprig text func map plus a toYaml helper. Sprig
// itself ships toJson but not toYaml; toYaml is convenient for capabilities
// (golangci-lint) that need to splat a settings map into YAML output.
func templateFuncs() template.FuncMap {
	funcs := sprig.TxtFuncMap()
	funcs["toYaml"] = func(v any) (string, error) {
		out, err := yaml.Marshal(v)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(out), "\n"), nil
	}
	return funcs
}

// runOutput captures the result of one tool invocation.
type runOutput struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// invocationCtx holds the per-invocation state used to expand argv.
type invocationCtx struct {
	ConfigFile string
	ReportPath string
	Files      []string
	Project    string
	Rule       *manifest.Rule
	Runtime    *runtimeCtx
}

// filterFiles applies AppliesToFiles.Extensions to the input list.
func (c *Capability) filterFiles(files []string) []string {
	if len(c.spec.Spec.AppliesToFiles.Extensions) == 0 {
		return files
	}
	var out []string
	for _, f := range files {
		for _, ext := range c.spec.Spec.AppliesToFiles.Extensions {
			if strings.HasSuffix(f, ext) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

// runBatch invokes the tool once with all rules. Used by per:batch list-mode
// capabilities (semgrep, gosec, golangci-lint, eslint).
func (c *Capability) runBatch(ctx context.Context, req engine.AnalysisRequest) (*runOutput, error) {
	files := c.filterFiles(req.Files)
	if len(files) == 0 {
		return nil, nil
	}

	rt, err := c.resolveRuntime(engineFiles{Files: files})
	if err != nil {
		return nil, err
	}
	if err := c.runPrechecks(rt, req.Rules); err != nil {
		return nil, err
	}

	var configPath string
	if c.spec.Spec.Run.ConfigFile != nil {
		path, err := c.renderConfigFile(req.Rules, rt)
		if err != nil {
			return nil, fmt.Errorf("render config_file: %w", err)
		}
		configPath = path
		defer os.Remove(configPath)
	}

	invFiles := files
	if c.spec.Spec.Run.FilesAsDirs {
		invFiles = uniqueDirs(files)
	}

	ictx := invocationCtx{
		ConfigFile: configPath,
		Files:      invFiles,
		Runtime:    rt,
	}

	batchView := batchViewOf(req.Rules, rt)

	out, err := c.execOnce(ctx, rt.BinaryPath, ictx, batchView)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// runPerFile invokes the tool once per target file. Used by per:file
// capabilities (gitleaks). The same set of rules is matched against each
// invocation's output; findings from all files are concatenated.
func (c *Capability) runPerFile(ctx context.Context, req engine.AnalysisRequest) ([]*runOutput, error) {
	files := c.filterFiles(req.Files)
	if len(files) == 0 {
		return nil, nil
	}

	rt, err := c.resolveRuntime(engineFiles{Files: files})
	if err != nil {
		return nil, err
	}

	outs := make([]*runOutput, 0, len(files))
	for _, f := range files {
		ictx := invocationCtx{
			Files:   []string{f},
			Runtime: rt,
		}
		view := struct {
			File        string
			ProjectRoot string
			Discovered  map[string]string
		}{File: f, ProjectRoot: rt.ProjectRoot, Discovered: rt.Discovered}
		out, err := c.execOnce(ctx, rt.BinaryPath, ictx, view)
		if err != nil {
			return nil, err
		}
		if out != nil {
			outs = append(outs, out)
		}
	}
	return outs, nil
}

// runPerRule invokes the tool once per rule. Used by scalar-mode capabilities
// (git, file-exists, file-contains).
func (c *Capability) runPerRule(ctx context.Context, req engine.AnalysisRequest) ([]ruleOutput, error) {
	files := c.filterFiles(req.Files)
	if len(files) == 0 && len(c.spec.Spec.AppliesToFiles.Extensions) > 0 {
		return nil, nil
	}

	rt, err := c.resolveRuntime(engineFiles{Files: files})
	if err != nil {
		return nil, err
	}

	project := projectRoot(files)

	results := make([]ruleOutput, 0, len(req.Rules))
	for i := range req.Rules {
		rule := req.Rules[i]
		ictx := invocationCtx{
			Files:   files,
			Project: project,
			Rule:    &rule,
			Runtime: rt,
		}
		view := ruleViewOf(rule)
		out, err := c.execOnce(ctx, rt.BinaryPath, ictx, view)
		results = append(results, ruleOutput{Rule: rule, Out: out, Err: err})
	}
	return results, nil
}

type ruleOutput struct {
	Rule manifest.Rule
	Out  *runOutput
	Err  error
}

// execOnce runs the tool exactly once. Handles timeout, exit-code policy,
// and stdout/report_file capture.
func (c *Capability) execOnce(ctx context.Context, bin string, ictx invocationCtx, templateView any) (*runOutput, error) {
	if c.spec.Spec.Run.Output == "report_file" {
		f, err := os.CreateTemp("", filenamePattern(c.spec.Spec.Run.ReportPathPattern))
		if err != nil {
			return nil, fmt.Errorf("create report file: %w", err)
		}
		ictx.ReportPath = f.Name()
		f.Close()
		defer os.Remove(ictx.ReportPath)
	}

	argv, err := c.expandArgv(ictx, templateView)
	if err != nil {
		return nil, fmt.Errorf("expand argv: %w", err)
	}

	timeout := c.spec.Spec.Run.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, bin, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("%s timed out after %s", c.spec.Spec.Requires.Binary, timeout)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			if !isOkExit(exitCode, c.spec.Spec.Run.OkExit) {
				return nil, fmt.Errorf("%s exited with code %d: %s", c.spec.Spec.Requires.Binary, exitCode, stderr.String())
			}
		} else {
			return nil, fmt.Errorf("%s error: %w\nstderr: %s", c.spec.Spec.Requires.Binary, runErr, stderr.String())
		}
	}

	out := &runOutput{Stderr: stderr.Bytes(), ExitCode: exitCode}
	if c.spec.Spec.Run.Output == "report_file" {
		data, _ := os.ReadFile(ictx.ReportPath)
		out.Stdout = data
	} else {
		out.Stdout = stdout.Bytes()
	}
	return out, nil
}

// isOkExit returns true if code is 0 (always allowed) or appears in the spec's
// ok_exit list.
func isOkExit(code int, ok []int) bool {
	if code == 0 {
		return true
	}
	for _, c := range ok {
		if c == code {
			return true
		}
	}
	return false
}

// renderConfigFile renders config_file.template against the rules and writes
// the output to a path resolved by resolveConfigPath. Caller owns cleanup.
func (c *Capability) renderConfigFile(rules []manifest.Rule, rt *runtimeCtx) (string, error) {
	cf := c.spec.Spec.Run.ConfigFile
	tmpl, err := template.New("config_file").Funcs(templateFuncs()).Parse(cf.Template)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	view := struct {
		Rules       []ruleView
		ProjectRoot string
		Discovered  map[string]string
		Pid         int
	}{
		Rules:       make([]ruleView, 0, len(rules)),
		ProjectRoot: rt.ProjectRoot,
		Discovered:  rt.Discovered,
		Pid:         os.Getpid(),
	}
	for _, r := range rules {
		view.Rules = append(view.Rules, ruleViewOf(r))
	}

	var rendered bytes.Buffer
	if err := tmpl.Execute(&rendered, view); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return writeConfigToPath(cf.Path, rt, rendered.Bytes())
}

// writeConfigToPath resolves the config_file.path against the runtime context
// and writes the rendered bytes. Two strategies are supported:
//
//   - "{tmp}/foo.json"               → os.CreateTemp using a derived pattern
//   - "{project_root}/.idio-foo-{pid}.json" → write directly to the resolved
//     path; runtime cleans up via the deferred os.Remove in runBatch
func writeConfigToPath(path string, rt *runtimeCtx, content []byte) (string, error) {
	// Substitute well-known placeholders.
	expanded := strings.ReplaceAll(path, "{project_root}", rt.ProjectRoot)
	expanded = strings.ReplaceAll(expanded, "{pid}", fmt.Sprintf("%d", os.Getpid()))
	for k, v := range rt.Discovered {
		expanded = strings.ReplaceAll(expanded, "{discovered."+k+"}", v)
	}

	if strings.HasPrefix(path, "{tmp}/") || !strings.ContainsAny(expanded, "/") {
		// Tempfile mode.
		f, err := os.CreateTemp("", filenamePattern(path))
		if err != nil {
			return "", fmt.Errorf("create temp config: %w", err)
		}
		if _, err := f.Write(content); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", fmt.Errorf("write config: %w", err)
		}
		f.Close()
		return f.Name(), nil
	}

	// Direct write mode (e.g. project_root/.idio-eslint-config-1234.mjs).
	if err := os.WriteFile(expanded, content, 0644); err != nil {
		return "", fmt.Errorf("write config %s: %w", expanded, err)
	}
	return expanded, nil
}

// filenamePattern turns "{tmp}/idio-foo.json" into "idio-foo-*.json".
func filenamePattern(path string) string {
	path = strings.TrimPrefix(path, "{tmp}/")
	dot := strings.LastIndex(path, ".")
	if dot < 0 {
		return path + "-*"
	}
	return path[:dot] + "-*" + path[dot:]
}

// expandArgv resolves the spec's argv into a real argv slice. Each entry can
// be:
//
//   - "{files}"  — expanded into multiple argv entries (one per target file)
//   - a Go template string rendered against templateView (sprig helpers
//     available)
//   - a literal containing well-known scalar tokens: {config_file},
//     {report_path}, {project}
//
// For per:rule capabilities with rule_argv_field set, the rule's input list
// is appended after the rendered static argv.
func (c *Capability) expandArgv(ictx invocationCtx, templateView any) ([]string, error) {
	out := make([]string, 0, len(c.spec.Spec.Run.Argv)+len(ictx.Files))
	for _, arg := range c.spec.Spec.Run.Argv {
		if arg == "{files}" {
			out = append(out, ictx.Files...)
			continue
		}
		// Go template substitution.
		if strings.Contains(arg, "{{") {
			t, err := template.New("arg").Funcs(templateFuncs()).Parse(arg)
			if err != nil {
				return nil, fmt.Errorf("parse argv template %q: %w", arg, err)
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, templateView); err != nil {
				return nil, fmt.Errorf("execute argv template %q: %w", arg, err)
			}
			arg = buf.String()
		}
		// Literal tokens.
		arg = strings.ReplaceAll(arg, "{config_file}", ictx.ConfigFile)
		arg = strings.ReplaceAll(arg, "{report_path}", ictx.ReportPath)
		arg = strings.ReplaceAll(arg, "{project}", ictx.Project)
		if ictx.Runtime != nil {
			arg = strings.ReplaceAll(arg, "{project_root}", ictx.Runtime.ProjectRoot)
			arg = strings.ReplaceAll(arg, "{pid}", fmt.Sprintf("%d", os.Getpid()))
			for k, v := range ictx.Runtime.Discovered {
				arg = strings.ReplaceAll(arg, "{discovered."+k+"}", v)
			}
		}
		out = append(out, arg)
	}

	// rule_argv_field: append the rule's named input list to argv.
	if ictx.Rule != nil && c.spec.Spec.Run.RuleArgvField != "" {
		raw, ok := ictx.Rule.Detector.Config[c.spec.Spec.Run.RuleArgvField]
		if ok {
			list, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("rule %q: input %q must be a list (got %T)", ictx.Rule.ID, c.spec.Spec.Run.RuleArgvField, raw)
			}
			for _, item := range list {
				out = append(out, fmt.Sprintf("%v", item))
			}
		}
	}

	return out, nil
}

// uniqueDirs collapses files to a deduplicated list of "<dir>/..." entries.
func uniqueDirs(files []string) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		dir := filepath.Dir(f)
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir+"/...")
		}
	}
	return dirs
}

// projectRoot picks a sensible project path from the input files.
func projectRoot(files []string) string {
	if len(files) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return "."
		}
		return cwd
	}
	abs, err := filepath.Abs(files[0])
	if err != nil {
		return files[0]
	}
	// If the input is a file, use its directory; if it's already a dir, keep it.
	info, err := os.Stat(abs)
	if err == nil && info.IsDir() {
		return abs
	}
	return filepath.Dir(abs)
}

// ruleView and fixView are the template-friendly projections of manifest.Rule.
type ruleView struct {
	ID       string
	Severity string
	Fix      fixView
	Inputs   map[string]any
}

type fixView struct {
	Message string
	Example string
}

func ruleViewOf(r manifest.Rule) ruleView {
	return ruleView{
		ID:       r.ID,
		Severity: string(r.Severity),
		Fix:      fixView{Message: r.Fix.Message, Example: r.Fix.Example},
		Inputs:   r.Detector.Config,
	}
}

// batchView is the projection used for batch-mode argv templates. It exposes
// aggregate rule data plus the resolved runtime context (project root,
// discovered files) so templates can interpolate them.
type batchView struct {
	Rules       []ruleView
	RuleIDs     []string
	ProjectRoot string
	Discovered  map[string]string
	Pid         int
}

func batchViewOf(rules []manifest.Rule, rt *runtimeCtx) batchView {
	v := batchView{
		Rules:       make([]ruleView, 0, len(rules)),
		RuleIDs:     make([]string, 0, len(rules)),
		ProjectRoot: rt.ProjectRoot,
		Discovered:  rt.Discovered,
		Pid:         os.Getpid(),
	}
	for _, r := range rules {
		v.Rules = append(v.Rules, ruleViewOf(r))
		if id, ok := r.Detector.Config["rule_id"].(string); ok && id != "" {
			v.RuleIDs = append(v.RuleIDs, id)
		}
	}
	return v
}
