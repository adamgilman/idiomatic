# Authoring a Capability

> A step-by-step walkthrough that builds a working capability from scratch.
> Audience: anyone who wants to add support for a CLI tool that idiomatic
> doesn't ship out of the box.

This guide builds a [Ruff](https://docs.astral.sh/ruff/) capability for
linting Python. By the end you'll have a single YAML file that:

- Discovers the `ruff` binary
- Filters input to `*.py` files
- Runs `ruff check --output-format json`
- Parses the output and routes findings back to your pack rules

For the formal field reference, see [`spec/capability.md`](../spec/capability.md).

## Prerequisites

- The capability YAML format is [documented in the spec](../spec/capability.md). Skim it first.
- An editor with [`yaml-language-server`](https://github.com/redhat-developer/yaml-language-server) is highly recommended — autocomplete and inline validation make the rest of this guide much easier.
- The CLI tool you're wrapping must already be installable. Idiomatic doesn't manage tool installs; it expects the binary on `PATH` (or somewhere a `discover` walk can find it).
- Read the tool's `--help` and study its JSON output format. Every capability is shaped by the tool it wraps.

## Step 1: Decide the binding shape

Two questions decide everything else:

1. **Does the tool emit a list of structured findings, or just one piece of output per invocation?**
   - List → `signal.shape: list` (semgrep, eslint, gosec, gitleaks, golangci-lint, ruff, eslint, ...)
   - One yes/no signal → `signal.shape: scalar` (git, file-exists, file-contains, ...)

2. **For a list-shaped tool: how often do you invoke it?**
   - Once for the whole rule set → `run.per: batch` (semgrep, gosec, golangci-lint, ruff, eslint)
   - Once per file → `run.per: file` (gitleaks, which scans each file separately)
   - Once per rule → `run.per: rule` (rare for list-shaped tools)

Ruff is `signal.shape: list` (it emits an array of issues) and `run.per: batch` (it accepts many files in one invocation). That's the most common shape.

## Step 2: Run the tool by hand and capture its output

```bash
echo 'x = 1' > /tmp/sample.py
ruff check --output-format json /tmp/sample.py
```

Output (truncated):

```json
[
  {
    "code": "F401",
    "message": "`x` is unused",
    "filename": "/tmp/sample.py",
    "location": { "row": 1, "column": 1 },
    "end_location": { "row": 1, "column": 6 }
  }
]
```

You're looking for:

- The **JSON path to the array of findings**. Here it's the top-level array, so `list: ""`.
- The **field that names the rule**. Here it's `code`.
- The **fields that locate the finding**. Here: `filename`, `location.row`, `location.column`, `end_location.row`, `end_location.column`.
- The **message field**. Here: `message`.
- Any **severity field**. Ruff doesn't include severity in the JSON output, so we'll inherit it from the pack rule.
- The **exit code** when issues are found. `ruff check` exits 1 when there are findings — that's our `ok_exit: [1]`.

If your tool's output structure is more complex (nested arrays, parent fields you need to project onto each finding), see the [list signal section in the spec](../spec/capability.md#list-signal). Two real-world examples:

- Eslint: `[{filePath, messages: [...]}]` — each message becomes a finding, `filePath` is projected onto every finding via `nested_list: messages` + `parent_fields: { file: filePath }`.
- gosec: `{Issues: [...]}` — top-level object with the array nested under `Issues`. Use `list: Issues`.

## Step 3: Decide the input contract

What does each rule need to supply to your capability? For ruff, the natural pattern is **one rule per ruff rule code**. The pack author writes:

```yaml
- id: py-no-unused-imports
  detector:
    capability: ruff
    version: "^1"
    config:
      rule_id: F401
```

So the capability declares one input: `rule_id` (string, required). The rule maps to the upstream tool's identifier.

We'll use `match_rule_by.strategy: by_input` so the runtime looks up each tool finding's `code` against `rule.config.rule_id`.

## Step 4: Decide which exit codes are findings

Most linters exit non-zero when they find issues. Read the tool's man page or test it:

```bash
ruff check /tmp/clean.py; echo "exit=$?"      # exit=0
ruff check /tmp/dirty.py; echo "exit=$?"      # exit=1
ruff check /tmp/missing.py; echo "exit=$?"    # exit=2 (real error)
```

Ruff: `0` clean, `1` issues, `2` error. Our `ok_exit: [1]` (0 is implicit).

## Step 5: Write the capability YAML

Create `capabilities/ruff.yaml`:

```yaml
# yaml-language-server: $schema=../../schemas/capability.schema.json
apiVersion: capabilities.idiomatic.dev/v1alpha1
kind: Capability
metadata:
  name: ruff
  version: 1.0.0
  description: Python linter and formatter (ruff) — wraps the binary, filters by Python files, routes findings back to rules

spec:
  applies_to_files:
    extensions: [".py"]

  requires:
    binary: ruff
    install: "uv tool install ruff"

  inputs:
    rule_id:
      type: string
      required: true
      description: ruff rule code (e.g. F401, E501)

  run:
    per: batch
    argv:
      - check
      - --output-format
      - json
      - --select
      - "{{ .RuleIDs | join \",\" }}"
      - "{files}"
    timeout: 60s
    ok_exit: [1]
    output: stdout

  signal:
    shape: list
    source: stdout
    format: json
    list: ""                  # ruff emits a top-level array
    match_rule_by:
      strategy: by_input
      field: rule_id
      from: code
    fields:
      file:     filename
      line:     location.row
      col:      location.column
      end_line: end_location.row
      end_col:  end_location.column
      message:  message
```

A few things to notice:

- `applies_to_files.extensions: [".py"]` — ruff only sees `.py` files, regardless of what the user passes to `idio scan`.
- `inputs.rule_id` is the only declared input. Rules supply just `{rule_id: F401}`.
- `argv` uses two placeholders: `{{ .RuleIDs | join "," }}` is a Go template that the runtime renders against the **batch view**, which exposes `.RuleIDs` as the deduplicated list of `rule_id` values across all matching rules. `{files}` expands as a single argv element to multiple entries (one per target file).
- `match_rule_by.strategy: by_input` plus `field: rule_id` and `from: code` tells the runtime: "for each finding in the JSON, look up its `code` field against `rule.config.rule_id`."
- We don't declare a `severity_map` because ruff doesn't emit severity. The runtime falls back to the rule's own severity from the pack.

## Step 6: Validate the YAML

```bash
python3 -c '
import json, yaml, sys
from jsonschema import Draft202012Validator
schema = json.load(open("schemas/capability.schema.json"))
doc = yaml.safe_load(open("capabilities/ruff.yaml"))
errs = list(Draft202012Validator(schema).iter_errors(doc))
for e in errs: print(f"{list(e.absolute_path)}: {e.message}")
sys.exit(1 if errs else 0)
'
```

If you used the `# yaml-language-server` comment at the top, your editor is
already doing this in the background.

## Step 7: Write a tiny pack to test against

Create `/tmp/ruff-test-pack.yaml`:

```yaml
# yaml-language-server: $schema=../../schemas/rule-pack.schema.json
apiVersion: rules.idiomatic.dev/v1alpha1
kind: RulePack
pack:
  id: ruff-test
  name: Ruff Test Pack
  version: 0.1.0
  description: Smoke test for the ruff capability.
  maintainer: me
rules:
  - id: py-no-unused-imports
    name: No unused imports
    description: Flags imports that are never used.
    rationale: Unused imports add noise and slow down readers.
    severity: warning
    detector:
      capability: ruff
      version: "^1"
      config:
        rule_id: F401
    applies_to: ["**/*.py"]
    fix:
      message: Remove the unused import.
```

## Step 8: Run end-to-end

```bash
echo 'import os' > /tmp/sample.py
idio scan /tmp/sample.py
```

(Make sure your `.idiomatic.yaml` references the ruff capability and the test pack.)

Expected output:

```
/tmp/sample.py:1:1: warning: `os` imported but unused [py-no-unused-imports]
```

If you get a capability-not-found error, double-check that `ruff.yaml` is
referenced in your `.idiomatic.yaml` `capabilities:` section and that the
apiVersion/kind/metadata are spelled correctly.

If you get `ruff binary is not installed`, install ruff and retry.

If you get findings with the wrong rule id, check that
`signal.match_rule_by.field` (the rule input) matches `signal.match_rule_by.from`
(the JSON field on each finding).

## Step 9: Reference the capability in `.idiomatic.yaml`

Add the new capability file to your project's `.idiomatic.yaml` under the
`capabilities:` section:

```yaml
capabilities:
  - repo: https://github.com/you/your-capabilities
    path:
      - capabilities/ruff.yaml
```

No Go code changes are needed. Capabilities are discovered dynamically from
the entries declared in `.idiomatic.yaml`.

## Step 10: Write tests

Smoke-test the new capability with both clean and dirty Python files:

```bash
echo 'import json' > /tmp/dirty.py
echo 'import json; print(json.dumps({}))' > /tmp/clean.py

idio scan /tmp/dirty.py /tmp/clean.py
```

Expected: one finding on `dirty.py`, none on `clean.py`.

For continuous validation, the existing test suite has examples of how to
exercise a capability against canned tool output. See
`internal/declarative/builtin_test.go` and `platform_test.go` for
patterns. The most useful pattern is a canned-output test: stub the binary
with a shell script that emits known JSON, then assert the parsed findings
match.

## Common patterns

### A tool that needs a config file (semgrep, golangci-lint, eslint)

Set `run.config_file.path` and `run.config_file.template`. The template
runs against a view of the active rule set and renders to a temp file.

```yaml
run:
  config_file:
    path: "{tmp}/idio-mytool-config.yaml"
    template: |
      rules:
      {{- range .Rules }}
        - id: {{ .ID }}
          ...
      {{- end }}
  argv:
    - --config
    - "{config_file}"
    - "{files}"
```

The runtime cleans up the temp file after the invocation. See
[`spec/capability.md#runconfig_file`](../spec/capability.md#runconfig_file)
for the full template view (`.Rules`, `.ProjectRoot`, `.Discovered`, `.Pid`).

### A tool whose binary lives in `node_modules/.bin/` (eslint)

Add `requires.discover`:

```yaml
requires:
  binary: eslint
  discover:
    strategy: walk_up_from_cwd
    relative: node_modules/.bin/eslint
    fallback_to_path: true
```

The runtime walks up from CWD looking for the relative path; the directory
where it finds the match becomes `{project_root}` for argv and config_file
templates.

### A tool that writes findings to a file instead of stdout (gitleaks)

Set `run.output: report_file` and `run.report_path_pattern`:

```yaml
run:
  output: report_file
  report_path_pattern: "{tmp}/idio-mytool-report.json"
  argv:
    - --report-path
    - "{report_path}"
    - "{files}"
```

`{report_path}` is the temp file the runtime allocates. After the tool
exits, the runtime reads it and feeds it to `signal` parsing.

### A tool that runs once per rule (git-style scalar)

Set `run.per: rule` and `signal.shape: scalar`. Each rule supplies its own
argv via `rule_argv_field` and queries the result with a `fire_when`
expression. See the `git`/`file-exists`/`file-contains` capabilities for
real examples.

### A tool with nested JSON output (eslint)

ESLint emits `[{filePath, messages: [...]}, ...]`. Use `nested_list` and
`parent_fields`:

```yaml
signal:
  shape: list
  list: ""
  nested_list: messages
  parent_fields:
    file: filePath
  match_rule_by:
    strategy: by_input
    field: rule
    from: ruleId
  fields:
    file:     parent.file       # read from the projected parent
    line:     line               # read from the inner item
    col:      column
    message:  message
```

## Versioning your capability

Pick a semver in `metadata.version` and bump it on every behavioral change:

| Bump | Examples |
|---|---|
| Patch | Fix a typo in `install` hint. Tighten `ok_exit`. |
| Minor | Add a new optional input. Add a new optional `signal.fields` mapping. |
| Major | Rename an input. Change `match_rule_by.strategy`. Change a `signal.fields` mapping. |

Pack rules pin via `detector.version`. A major bump fails every rule that
pinned to the previous major — that's the point.

## Publishing

Capabilities are loaded via git clone from HTTPS URLs declared in
`.idiomatic.yaml`. To publish a capability for community use, push it to a
public git repo:

```
my-capabilities/
├── capabilities/
│   ├── ruff.yaml
│   └── black.yaml
└── README.md
```

Consumers reference it in their `.idiomatic.yaml`:

```yaml
capabilities:
  - repo: https://github.com/me/my-capabilities
    path:
      - capabilities/ruff.yaml
      - capabilities/black.yaml
```

For private capabilities, push to a private git repo and rely on git auth.

## Debugging tips

- If the tool runs but produces zero findings, check `signal.list` and `signal.match_rule_by.from` against the actual JSON output. Use `jq` or `python3 -c 'import json,sys; print(json.load(sys.stdin))' < canned.json` to inspect the structure.
- If the runtime errors with `expand argv`, you have an unbalanced template. Run the template through `text/template` directly (or just simplify it) until it parses.
- If `match_rule_by.strategy: by_input` produces no matches, log the extracted value vs. the rule's input value — they're often case- or prefix-sensitive.
- Use `idio scan -f sarif ...` to see the full structured output, including the rules that loaded but didn't fire.
