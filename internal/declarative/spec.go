// SPDX-License-Identifier: Apache-2.0

// spec.go — Go struct definitions mirroring the capability YAML schema.
//
//   - Layout is Kubernetes-shaped: apiVersion + kind for versioning, metadata
//     for identity, spec for behavior. New behavioral fields go under spec.
//   - validateSpec (in loader.go) enforces structural correctness at load time
//     and fills in defaults (e.g. run.per defaults to "batch", signal.format
//     to "json"). The structs here are the post-parse, pre-validation shape.
//   - Two signal shapes drive the entire dispatch split: "list" (batch tool
//     output walked as JSON array) vs "scalar" (per-rule invocation queried
//     by fire_when expressions). Every other struct fans out from this.

// Package declarative implements YAML-driven capabilities. A CapabilitySpec
// describes how to wrap a CLI tool: how rules supply inputs, how to invoke the
// tool, and how to project the tool's output into a typed signal.
//
// See /root/.claude/plans/dynamic-napping-willow.md for the design.
package declarative

import "time"

const (
	// CapabilityAPIVersion is the schema version of the capability YAML
	// format. Bumping this implies a structural break — readers should
	// reject any capability whose apiVersion they don't recognize. New
	// fields that are backwards-compatible additions do NOT require an
	// apiVersion bump; only renames, removals, or semantic changes do.
	CapabilityAPIVersion = "capabilities.idiomatic.dev/v1alpha1"

	// CapabilityKind is the discriminator a capability YAML must declare.
	CapabilityKind = "Capability"
)

// CapabilitySpec is the parsed YAML for a single capability. The layout is
// Kubernetes-shaped: a stable apiVersion + kind, identity in metadata,
// behavior in spec.
//
// Adding a new field to the body? Put it under spec, not at the top level.
// Adding a new identity field (label, annotation, dependency declaration)?
// Put it under metadata.
type CapabilitySpec struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   CapabilityMetadata     `yaml:"metadata"`
	Spec       CapabilitySpecBody     `yaml:"spec"`
}

// CapabilityMetadata holds identity fields. Name is the routing key (rules
// reference it by `detector.capability`). Version is the semver of THIS
// CAPABILITY (not the underlying CLI tool); rules can pin to a major or
// range via `detector.version`. Description is informational.
type CapabilityMetadata struct {
	Name        string `yaml:"name"`
	Version     string `yaml:"version"`
	Description string `yaml:"description,omitempty"`
}

// CapabilitySpecBody is the behavior half of the capability — the bits that
// describe HOW the tool runs and how its output becomes findings.
type CapabilitySpecBody struct {
	AppliesToFiles AppliesToFiles        `yaml:"applies_to_files,omitempty"`
	Requires       RequiresSpec          `yaml:"requires"`
	Inputs         map[string]InputField `yaml:"inputs,omitempty"`
	Run            RunSpec               `yaml:"run"`
	Signal         SignalSpec            `yaml:"signal"`
}

// AppliesToFiles tells the file router which files this capability cares
// about. Used by both the scan path (filter req.Files before invoking the
// tool) and the claudehook path (decide which capabilities to dispatch a
// changed file to).
type AppliesToFiles struct {
	Extensions []string `yaml:"extensions,omitempty"` // exact suffix match, e.g. [".go", ".ts"]; empty = all files
}

// RequiresSpec declares the binary the capability wraps and any optional
// pre-invocation discovery / preflight checks.
type RequiresSpec struct {
	Binary  string `yaml:"binary"`
	Install string `yaml:"install,omitempty"`

	// Discover overrides the default `exec.LookPath` behavior. When set, the
	// runtime walks up directories looking for a relative path. The directory
	// where the binary is found is exposed as the `{project_root}` placeholder
	// for argv, config_file paths, and template rendering. Used by JS-style
	// capabilities (eslint) that prefer locally-installed binaries.
	Discover *DiscoverBinarySpec `yaml:"discover,omitempty"`

	// DiscoverFiles is a list of file lookups exposed to templates as
	// `{discovered.<var>}` placeholders or `{{ .Discovered.<var> }}` template
	// values. Used by capabilities that need context files (e.g. eslint's
	// tsconfig.json) discovered at runtime.
	DiscoverFiles []DiscoverFileSpec `yaml:"discover_files,omitempty"`

	// Precheck is a list of preflight conditions evaluated before invocation.
	// The first failing precheck aborts with its error message. Used by
	// capabilities that need to verify plugins or other dependencies are
	// installed before running the tool.
	Precheck []PrecheckSpec `yaml:"precheck,omitempty"`
}

// DiscoverBinarySpec describes how to locate a binary that may not be on PATH.
type DiscoverBinarySpec struct {
	// Strategy is the discovery mode. Currently only "walk_up_from_cwd" is
	// implemented: walk parent directories from CWD looking for the relative
	// path. The dir containing the match becomes {project_root}.
	Strategy string `yaml:"strategy"`

	// Relative is the path under each ancestor directory to test for.
	// Example: "node_modules/.bin/eslint".
	Relative string `yaml:"relative"`

	// FallbackToPath: if walk-up fails, fall back to exec.LookPath(binary).
	FallbackToPath bool `yaml:"fallback_to_path,omitempty"`
}

// DiscoverFileSpec describes one file lookup exposed as a template variable.
type DiscoverFileSpec struct {
	// Var is the variable name. Templates reference it as
	// {{ .Discovered.<var> }} and argv references it as {discovered.<var>}.
	Var string `yaml:"var"`

	// Name is the file name to look for at each ancestor directory.
	Name string `yaml:"name"`

	// WalkUpFrom is "cwd" (default). The walk starts from this directory and
	// stops at the filesystem root.
	WalkUpFrom string `yaml:"walk_up_from,omitempty"`

	// Optional means the variable defaults to "" if no match is found
	// (instead of aborting with an error).
	Optional bool `yaml:"optional,omitempty"`
}

// PrecheckSpec is one preflight condition.
type PrecheckSpec struct {
	// Kind is the check type. Currently only "file_exists" is implemented.
	Kind string `yaml:"kind"`

	// Path is the file path to check. Resolved against {project_root} and
	// {{ .Value }} (when ForEachUniqueInput is set).
	Path string `yaml:"path"`

	// ForEachUniqueInput names a rule input field. The precheck runs once
	// per unique non-empty value seen across all matching rules; templates
	// in Path and ErrorMessage can reference {{ .Value }}.
	ForEachUniqueInput string `yaml:"for_each_unique_input,omitempty"`

	// SkipValues are values that should be ignored by ForEachUniqueInput
	// (e.g. plugins that ship inline with the host capability).
	SkipValues []string `yaml:"skip_values,omitempty"`

	// ErrorMessage is the user-facing error if the precheck fails. Templated
	// against the same view as Path.
	ErrorMessage string `yaml:"error_message"`
}

// InputField is one slot in the capability's input contract.
type InputField struct {
	Type        string `yaml:"type"` // string | list | map | any
	Required    bool   `yaml:"required,omitempty"`
	Description string `yaml:"description,omitempty"`
}

// RunSpec describes how to invoke the tool.
type RunSpec struct {
	// Per controls invocation count: "batch" (default) runs all matching rules
	// in one invocation; "rule" invokes once per rule; "file" invokes once per
	// target file.
	Per        string          `yaml:"per,omitempty"`
	ConfigFile *ConfigFileSpec `yaml:"config_file,omitempty"`
	Argv       []string        `yaml:"argv"`
	Timeout    time.Duration   `yaml:"timeout"`
	OkExit     []int           `yaml:"ok_exit,omitempty"`
	Output     string          `yaml:"output,omitempty"` // stdout | report_file
	// ReportPathPattern is the temp filename pattern for report_file capture.
	// Used only when Output == "report_file". Replaces {report_path} in argv.
	ReportPathPattern string `yaml:"report_path_pattern,omitempty"`
	// FilesAsDirs translates input files into a deduplicated list of
	// "<dir>/..." entries (golangci-lint convention).
	FilesAsDirs bool `yaml:"files_as_dirs,omitempty"`
	// RuleArgvField (per:rule only) names a rule input field whose list value
	// is appended to argv at execution time. Used by capabilities like `git`
	// where each rule supplies its own subcommand.
	RuleArgvField string `yaml:"rule_argv_field,omitempty"`
}

// ConfigFileSpec describes a temp file generated from the matching rules and
// referenced by argv via the {config_file} placeholder.
type ConfigFileSpec struct {
	Path     string `yaml:"path"`
	Template string `yaml:"template"`
}

// SignalSpec describes how to project the tool's output into structured data
// that can be matched against rules.
type SignalSpec struct {
	// Shape is "list" (each item maps to a rule by ID) or "scalar" (single
	// object queried per-rule by fire_when expressions).
	Shape  string `yaml:"shape"`
	Source string `yaml:"source,omitempty"` // stdout (default) | report_file
	Format string `yaml:"format,omitempty"` // json | text

	// List-mode fields
	List string `yaml:"list,omitempty"`

	// NestedList enables a two-level walk: List selects an outer array,
	// then NestedList selects a nested array on each outer item. Each
	// inner item becomes a finding. ParentFields exposes outer-item fields
	// onto the inner item via the "parent.<alias>" gjson path. Used by
	// eslint, whose JSON shape is [{filePath, messages: [...]}, ...].
	NestedList   string            `yaml:"nested_list,omitempty"`
	ParentFields map[string]string `yaml:"parent_fields,omitempty"`

	MatchRuleBy *MatchRuleBy      `yaml:"match_rule_by,omitempty"`
	Fields      map[string]string `yaml:"fields,omitempty"`
	SeverityMap map[string]string `yaml:"severity_map,omitempty"`

	// Scalar-mode fields
	ScalarFields map[string]ScalarField `yaml:"scalar_fields,omitempty"`
	// FixedFile is the path emitted in findings for scalar-mode capabilities
	// that don't have file context (e.g. git checks). May contain
	// {project} placeholder.
	FixedFile string `yaml:"fixed_file,omitempty"`
}

// MatchRuleBy describes how a list-mode signal item names the rule it belongs to.
type MatchRuleBy struct {
	// Strategy: "by_id" (default) — match the rule whose pack id equals the
	// extracted value; "by_input" — match the rule whose inputs[Field] equals
	// the extracted value; "linter_contains" — multiple rules can share the
	// same primary id (linter), disambiguate by substring-matching SubruleField
	// against the entry's MatchIn field; "by_capability" — every finding maps
	// to the first pack rule using this capability (ignores the JSON item;
	// used by single-rule per-linter capabilities like errcheck).
	Strategy string `yaml:"strategy,omitempty"`

	From      string `yaml:"from"`                // gjson path on the item (not required for by_capability)
	Transform string `yaml:"transform,omitempty"` // identity (default) | tail_after_dot | prefix_before_colon

	// by_input strategy
	Field string `yaml:"field,omitempty"` // rule input field to match against

	// linter_contains strategy
	SubruleField string `yaml:"subrule_field,omitempty"` // rule input field carrying the sub-rule name
	MatchIn      string `yaml:"match_in,omitempty"`      // gjson path containing the substring to match
}

// ScalarField describes one field exposed on a scalar signal.
type ScalarField struct {
	From      string `yaml:"from"`                // stdout | stderr | exit
	Transform string `yaml:"transform,omitempty"` // identity (default) | trim
}
