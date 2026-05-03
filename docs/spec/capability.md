# Capability Specification

> Reference for the YAML format used to describe an idiomatic capability.
> Audience: anyone authoring a capability — for the official set, a community
> registry, or a private one.

The capability YAML format is a public specification. Anyone can author
capabilities — community contributors, companies wrapping internal tools,
individuals adding support for a CLI we don't ship out of the box. All
capabilities use the same schema, the same validation, and the same scanner
regardless of source.

This document is the **reference** for the format. For a step-by-step
walkthrough that builds a new capability from scratch, see
[`authoring/capability.md`](../authoring/capability.md).

## Status and stability

| Property | Value |
|---|---|
| Schema name | `capabilities.idiomatic.dev/v1alpha1` |
| JSON Schema | [`schemas/capability.schema.json`](../../schemas/capability.schema.json) |
| Stability | Alpha. Breaking changes bump the schema name (`v1alpha2`, `v1beta1`, ...). Backwards-compatible additions do not. |
| Versioning | Each capability YAML declares its **own** semver in `metadata.version`. This is independent of the schema version. |

## Editor support

Add this comment as the first line of any capability YAML and your editor
will validate the document and offer autocomplete (requires
[`yaml-language-server`](https://github.com/redhat-developer/yaml-language-server),
which ships with the Red Hat YAML extension for VS Code, the YAML extensions
for Neovim/Helix, and most other LSP-aware editors):

```yaml
# yaml-language-server: $schema=../../schemas/capability.schema.json
```

For local development inside this repo, point at the relative path instead:

```yaml
# yaml-language-server: $schema=../schemas/capability.schema.json
```

## Validating outside an editor

The schema is plain JSON Schema draft 2020-12. Any validator works. Two
common choices:

```bash
# Python — pip install jsonschema pyyaml
python3 -c '
import json, yaml, sys
from jsonschema import Draft202012Validator
schema = json.load(open("schemas/capability.schema.json"))
doc = yaml.safe_load(open(sys.argv[1]))
errs = list(Draft202012Validator(schema).iter_errors(doc))
for e in errs: print(f"{list(e.absolute_path)}: {e.message}")
sys.exit(1 if errs else 0)' capabilities/semgrep.yaml

# JavaScript — npm install -g ajv-cli ajv-formats
ajv validate \
  -s schemas/capability.schema.json \
  -d capabilities/semgrep.yaml \
  --spec=draft2020 --strict=false
```

CI for community capability repos can use either to gate on schema conformance
before publishing.

---

## Document structure

A capability is a Kubernetes-shaped document: a stable `apiVersion` + `kind`,
identity in `metadata`, behavior in `spec`. The minimum viable capability is:

```yaml
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata:
  name: my-tool
  version: 1.0.0
spec:
  requires:
    binary: my-tool
  run:
    argv: [--json, "{files}"]
  signal:
    shape: list
    list: results
    match_rule_by:
      from: id
    fields:
      file: path
      line: line
      message: message
```

Every field is documented in the sections below.

---

## Top-level fields

| Field | Type | Required | Description |
|---|---|---|---|
| `apiVersion` | string | yes | Always `capabilities.idiomatic.dev/v1alpha1`. Unrecognized values are rejected at load time. |
| `kind` | string | yes | Always `Capability`. |
| `metadata` | object | yes | Identity. See [Metadata](#metadata). |
| `spec` | object | yes | Behavior. See [Spec](#spec). |

---

## Metadata

```yaml
metadata:
  name: semgrep
  version: 1.0.0
  description: Semantic pattern matcher for many languages
```

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Unique identifier within a registry. Lowercase, hyphenated, alphanumeric (`^[a-z0-9][a-z0-9-]*[a-z0-9]$`). This is the **routing key**: rules reference the capability by this name in `detector.capability`. |
| `version` | string | yes | Semver of this capability. Bump on every behavioral change (see [Versioning](#versioning) below). |
| `description` | string | no | One-liner. |

### Versioning

The capability's version is independent of the schema's `apiVersion` and of
the underlying tool's version. It tracks **how this capability wraps the
tool**.

| Bump | When |
|---|---|
| Patch (`1.0.0` → `1.0.1`) | Bug fix that doesn't change rule behavior |
| Minor (`1.0.0` → `1.1.0`) | Backwards-compatible additions: new optional inputs, new optional spec fields |
| Major (`1.0.0` → `2.0.0`) | Breaking changes: renamed inputs, changed `match_rule_by` strategy, changed signal projection |

Rules pin to a version constraint via `detector.version`. The CLI rejects
mismatches **before any tool runs**, so a stale rule pinned to `^1` cannot
silently match against a `2.x` capability.

---

## Spec

```yaml
spec:
  applies_to_files: { ... }   # which files this capability cares about
  requires:        { ... }    # binary, install hint, optional discovery and prechecks
  inputs:          { ... }    # per-rule input contract
  run:             { ... }    # how to invoke the tool
  signal:          { ... }    # how to project tool output into findings
```

### `spec.applies_to_files`

Tells the file router which files this capability cares about. Used by both
the scan path (filter `req.Files` before invoking the tool) and the
claudehook router (decide which capabilities to dispatch a changed file to).

Set `extensions` to filter by file suffix. Omit `applies_to_files` entirely to receive every file (used by global scanners like gitleaks and git).

```yaml
# Filter by extension:
applies_to_files:
  extensions: [".go"]

# Receive every file (omit applies_to_files):
# applies_to_files is not set
```

| Field | Type | Description |
|---|---|---|
| `extensions` | string[] | Each entry is a literal suffix including the leading dot. Omit to receive all files. |

### `spec.requires`

Declares the binary the capability wraps and any optional pre-invocation
discovery / preflight checks.

```yaml
requires:
  binary: semgrep
  install: "uv tool install semgrep"

  # Optional walk-up binary discovery (eslint-style)
  discover:
    strategy: walk_up_from_cwd
    relative: node_modules/.bin/eslint
    fallback_to_path: true

  # Optional walk-up file lookups exposed as {discovered.<var>}
  discover_files:
    - var: tsconfig
      name: tsconfig.json
      walk_up_from: cwd
      optional: true

  # Optional preflight checks
  precheck:
    - kind: file_exists
      path: "{project_root}/node_modules/{{ .Value }}"
      for_each_unique_input: plugin
      skip_values: ["@typescript-eslint/eslint-plugin"]
      error_message: |
        ESLint plugin {{ .Value }} not found in {project_root}/node_modules.
        Install with: npm install --save-dev {{ .Value }}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `binary` | string | yes | Name of the executable to invoke. Looked up via `exec.LookPath` unless `discover` is set. |
| `install` | string | no | Human-readable install hint, surfaced when the binary is not found. |
| `discover` | object | no | Walk-up binary discovery. See below. |
| `discover_files` | object[] | no | Walk-up file lookups exposed as template variables. |
| `precheck` | object[] | no | Preflight conditions evaluated before invocation. |

#### `requires.discover`

Overrides the default `exec.LookPath` behavior. The runtime walks parent
directories from CWD looking for the relative path. The directory containing
the match becomes the `{project_root}` placeholder, available to argv,
config_file paths, and template rendering.

| Field | Type | Required | Description |
|---|---|---|---|
| `strategy` | string | yes | Currently only `walk_up_from_cwd` is implemented. |
| `relative` | string | yes | Path under each ancestor directory to test for. |
| `fallback_to_path` | bool | no | If walk-up fails, fall back to `exec.LookPath(binary)`. Default `false`. |

#### `requires.discover_files`

Each entry is a file lookup exposed to templates as `{{ .Discovered.<var> }}`
and to argv as `{discovered.<var>}`.

| Field | Type | Required | Description |
|---|---|---|---|
| `var` | string | yes | Variable name. Must be a valid identifier (`[A-Za-z_][A-Za-z0-9_]*`). |
| `name` | string | yes | File name to look for at each ancestor directory. |
| `walk_up_from` | string | no | Where the walk starts. Currently only `cwd`. |
| `optional` | bool | no | If true, the variable defaults to `""` when no match is found instead of aborting. |

#### `requires.precheck`

Preflight conditions evaluated before invocation. The first failing precheck
aborts with its error message.

| Field | Type | Required | Description |
|---|---|---|---|
| `kind` | string | yes | Currently only `file_exists` is implemented. |
| `path` | string | yes | File path to check. Resolved against `{project_root}` and `{{ .Value }}` (when `for_each_unique_input` is set). |
| `for_each_unique_input` | string | no | Names a rule input field. The precheck runs once per unique non-empty value seen across all matching rules; templates can reference `{{ .Value }}`. |
| `skip_values` | string[] | no | Values that `for_each_unique_input` should ignore. |
| `error_message` | string | yes | User-facing error if the check fails. Templated against the same view as `path`. |

### `spec.inputs`

Declares the slots a rule must (or may) fill. Each key is a slot name; the
value declares its type and required/optional status. Rules supply these
values via `detector.config` in the rule pack.

```yaml
inputs:
  language:
    type: string
    required: true
    description: Target language (go, typescript, python, ...)
  pattern:
    type: any
    description: Single semgrep pattern
  patterns:
    type: any
    description: Compound pattern (list of pattern fragments)
```

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | string | yes | One of `string`, `list`, `map`, `any`. `any` accepts any YAML value (used when the rule supplies an opaque structure that the capability template splats through). |
| `required` | bool | no | If true, rules must supply this input. Default `false`. |
| `description` | string | no | Human-readable explanation of what this input is for. |

### `spec.run`

How to invoke the tool. The dispatch is determined by `per`: `batch` (one
invocation for all matching rules), `rule` (one invocation per rule), or
`file` (one invocation per target file).

```yaml
run:
  per: batch
  config_file:
    path: "{tmp}/idio-semgrep-rules.yaml"
    template: |
      rules:
      {{- range .Rules }}
        - id: {{ .ID }}
          ...
      {{- end }}
  argv:
    - scan
    - --config
    - "{config_file}"
    - --json
    - "{files}"
  timeout: 60s
  ok_exit: [1]
  output: stdout
```

| Field | Type | Required | Description |
|---|---|---|---|
| `per` | string | no | Invocation strategy: `batch` (default), `rule`, or `file`. |
| `config_file` | object | no | Optional config file generated from the rules. See below. |
| `argv` | string[] | yes | Argv passed to the binary. Each entry can be a literal, a Go template, or a placeholder (see [Argv placeholders](#argv-placeholders)). |
| `rule_argv_field` | string | no | (`per: rule` only) Names a rule input field whose list value is appended to argv. Used by capabilities like `git` where each rule supplies its own subcommand. |
| `timeout` | duration | no | Go duration string (`60s`, `2m`). Default 60s. |
| `ok_exit` | int[] | no | Non-zero exit codes that should be treated as success (most linters return 1 when they find issues). Exit code 0 is always treated as success. |
| `output` | string | no | Where to read tool output from: `stdout` (default) or `report_file`. |
| `report_path_pattern` | string | required if `output: report_file` | Filename pattern for the temp report file (uses `os.CreateTemp` semantics). |
| `files_as_dirs` | bool | no | If true, target files are translated into a deduplicated list of `<dir>/...` entries. Used by tools (golangci-lint) that take directories instead of files. |

#### `run.config_file`

Describes a temp file generated from the matching rules. The path placeholder
controls where it's written:

- `{tmp}/foo.json` → `os.CreateTemp` produces a temp file (default lifecycle)
- `{project_root}/.idio-foo-{pid}.mjs` → write directly to the discovered project root (for tools like ESLint that need the config alongside `node_modules/`)

Both are cleaned up by the runtime after the invocation.

| Field | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Where to write the file. Supports `{tmp}`, `{project_root}`, `{pid}`, `{discovered.<var>}` placeholders. |
| `template` | string | yes | Go `text/template` body with sprig helpers (`toYaml`, `toJson`, `quote`, `indent`, `nindent`, `default`, `list`, `append`, `range`, `if`, ...). The view exposes `.Rules`, `.ProjectRoot`, `.Discovered`, `.Pid`. |

#### Argv placeholders

In addition to Go template syntax, argv entries support these literal
placeholders that the runtime substitutes at invocation time:

| Placeholder | Replaced with |
|---|---|
| `{config_file}` | The path to the rendered config file (only when `run.config_file` is set). |
| `{report_path}` | The path to the temp report file (only when `output: report_file`). |
| `{project_root}` | The directory where the binary was discovered (only when `requires.discover` is set). |
| `{pid}` | Current process ID. |
| `{discovered.<var>}` | Value of a `discover_files` variable. |
| `{files}` | Expands as a single argv entry into multiple entries — one per target file. Must appear as an entire argv element, not embedded. |

#### Templates

Go templates run **before** literal placeholder substitution. The view passed
to argv templates depends on `run.per`:

- `per: batch` → `batchView` with `.Rules`, `.RuleIDs`, `.ProjectRoot`, `.Discovered`, `.Pid`
- `per: rule`  → `ruleView` with `.ID`, `.Severity`, `.Fix`, `.Inputs`
- `per: file`  → `{ File, ProjectRoot, Discovered }`

Sprig helpers are available: `toYaml`, `toJson`, `quote`, `default`,
`indent`, `nindent`, `list`, `append`, `keys`, `sortAlpha`, `range`, `if`,
`with`, `eq`, `ne`, and the rest of the [sprig text func map](https://masterminds.github.io/sprig/).

### `spec.signal`

How the tool's output becomes findings. Two shapes: `list` (the tool emits a
list of structured findings, each item maps back to a rule) or `scalar` (the
tool's output is queried per-rule by a `fire_when` expression).

#### List signal

```yaml
signal:
  shape: list
  source: stdout
  format: json
  list: results              # gjson path to the outer array; "" for top-level array
  match_rule_by:
    strategy: by_id          # by_id | by_input | by_capability
    from: check_id           # gjson path on the inner item
    transform: tail_after_dot
  fields:
    file: path
    line: start.line
    col: start.col
    end_line: end.line
    end_col: end.col
    message: extra.message
    severity: extra.severity
  severity_map:
    ERROR: error
    WARNING: warning
    INFO: info
```

| Field | Type | Required | Description |
|---|---|---|---|
| `shape` | string | yes | `list`. |
| `source` | string | no | Where to read output from: `stdout` (default) or `report_file`. |
| `format` | string | no | Currently only `json`. |
| `list` | string | no | gjson path to the outer array. Empty string = top-level array. The trailing `.#` is optional. |
| `nested_list` | string | no | Optional second-level walk: select a nested array on each outer item. Each inner item becomes a finding. |
| `parent_fields` | object | no | Outer-item fields exposed to inner items via `parent.<alias>` paths in `signal.fields`. Only used with `nested_list`. |
| `match_rule_by` | object | yes | How each item names the rule it belongs to. See below. |
| `fields` | object | no | Maps Finding fields (`file`, `line`, `col`, `end_line`, `end_col`, `message`, `severity`) to gjson paths on the inner item. Use `parent.<alias>` to read from a projected parent field. |
| `severity_map` | object | no | Maps upstream severity strings to idiomatic severities (`error`/`warning`/`info`). |

##### `match_rule_by` strategies

| Strategy | Behavior |
|---|---|
| `by_id` (default) | Pack rule id equals the value extracted from `from`. |
| `by_input` | Extracted value matches `rule.config[field]`. Used when the rule references the upstream tool's id (e.g. `gosec` rule_id `G101`). |
| `by_capability` | Every finding emitted by the tool routes to the first pack rule that targets this capability. Used by single-rule per-linter capabilities (`errcheck`, `nakedret`, etc.) where the linter has no sub-rules to disambiguate. |

### `by_capability`

Maps every finding emitted by the tool to the first pack rule that targets this
capability. Used by single-rule per-linter capabilities (`errcheck`, `nakedret`,
etc.) where the linter has no sub-rules to disambiguate.

If multiple pack rules target the same single-rule capability (e.g. two
different packs both wrap `errcheck`), findings attribute to the first by
deterministic load order.

This strategy ignores the JSON item entirely — `match_rule_by.from` is not
required (and is ignored if present).

##### Transforms

| Transform | Behavior |
|---|---|
| `identity` (default) | Use the value as-is. |
| `tail_after_dot` | Strip everything up to the last `.`. Used by semgrep, whose check_ids are prefixed with the temp config dir name. |
| `prefix_before_colon` | Returns the substring before the first `:` in the input, with surrounding whitespace trimmed. Used by linters like `revive` that embed the rule name as a prefix in their output text — `"exported: should have a comment"` becomes `"exported"`. |

### `prefix_before_colon`

Returns the substring before the first `:` in the input, with surrounding
whitespace trimmed. Used by linters like `revive` that embed the rule name as
a prefix in their output text — `"exported: should have a comment"` becomes
`"exported"`.

#### Scalar signal

```yaml
signal:
  shape: scalar
  scalar_fields:
    stdout:    { from: stdout, transform: trim }
    stderr:    { from: stderr, transform: trim }
    exit_code: { from: exit }
  fixed_file: "{project}/.git/HEAD"
```

Used by capabilities (`git`, `file-exists`, `file-contains`) where the tool
emits one piece of output per invocation and rules query it with a
`fire_when` expression. See the rule pack spec for the expression grammar.

| Field | Type | Required | Description |
|---|---|---|---|
| `shape` | string | yes | `scalar`. |
| `scalar_fields` | object | no | Named projections of the scalar output. |
| `fixed_file` | string | no | Synthetic file path emitted in findings. Supports `{project}` and `{{ .Inputs.<field> }}` substitutions. |

##### `scalar_fields`

Each entry pulls from one of `stdout`, `stderr`, or `exit` and applies an
optional transform. The standard names `stdout`, `stderr`, `exit_code` are
always exposed automatically; you only need to declare them here to apply a
transform (e.g. `trim`).

| Field | Type | Description |
|---|---|---|
| `from` | string | One of `stdout`, `stderr`, `exit`. |
| `transform` | string | `identity` (default) or `trim`. |

---

## Two complete examples

### List-mode capability (semgrep)

```yaml
# yaml-language-server: $schema=../../schemas/capability.schema.json
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata:
  name: semgrep
  version: 1.0.0
  description: Semantic pattern matcher for many languages

spec:
  requires:
    binary: semgrep
    install: "uv tool install semgrep"

  inputs:
    language: { type: string, required: true }
    pattern:  { type: any }
    patterns: { type: any }

  run:
    per: batch
    config_file:
      path: "{tmp}/idio-semgrep-rules.yaml"
      template: |
        rules:
        {{- range .Rules }}
          - id: {{ .ID }}
            languages: [{{ index .Inputs "language" }}]
            severity: {{ .Severity | upper }}
            message: {{ .Fix.Message | quote }}
        {{- range $key := keys .Inputs | sortAlpha }}
        {{- if ne $key "language" }}
            {{ $key }}: {{ index $.Inputs $key | toJson }}
        {{- end }}
        {{- end }}
        {{- end }}
    argv:
      - scan
      - --config
      - "{config_file}"
      - --json
      - --no-git-ignore
      - "{files}"
    timeout: 60s
    ok_exit: [1]
    output: stdout

  signal:
    shape: list
    list: results
    match_rule_by:
      from: check_id
      transform: tail_after_dot
    fields:
      file:    path
      line:    start.line
      col:     start.col
      message: extra.message
      severity: extra.severity
    severity_map:
      ERROR: error
      WARNING: warning
      INFO: info
```

### Scalar-mode capability (git)

```yaml
# yaml-language-server: $schema=../../schemas/capability.schema.json
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata:
  name: git
  version: 1.0.0
  description: Wrapper around the git CLI

spec:
  requires:
    binary: git
    install: "your system package manager"

  inputs:
    argv:      { type: list,   required: true }
    fire_when: { type: string, required: true }

  run:
    per: rule
    argv: []
    rule_argv_field: argv
    timeout: 5s
    ok_exit: [0, 1, 128]
    output: stdout

  signal:
    shape: scalar
    scalar_fields:
      stdout:    { from: stdout, transform: trim }
      stderr:    { from: stderr, transform: trim }
      exit_code: { from: exit }
    fixed_file: "{project}/.git/HEAD"
```

A rule using this capability looks like:

```yaml
- id: no-work-on-main
  capability: git
  version: "^1"
  config:
    argv: [branch, --show-current]
    fire_when: 'signal.stdout matches "^(main|master)$"'
```

See the [rule pack spec](rule-pack.md) for the rule format and the `fire_when` expression grammar.
