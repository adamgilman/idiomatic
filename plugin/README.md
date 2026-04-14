# plugin — Claude Code Plugin

The `idiomatic` Claude Code plugin provides deterministic rule enforcement during agentic code generation. After every file edit, a PostToolUse hook runs the analyzer and feeds findings back into Claude's context for immediate correction.

## How It Works

```
Claude edits foo.go
  → PostToolUse hook fires
  → idio scan --format=claude-hook (reads hook JSON from stdin)
  → discovers .idiomatic.yaml, analyzes the edited file
  → wraps findings in additionalContext
  → Claude reads findings via the skill instructions
  → Claude applies fixes
  → hook fires again → repeat until clean or 3-iteration limit
```

## Directory Structure

```
plugin/
├── .claude-plugin/
│   └── plugin.json           # Plugin manifest (name, version, author)
├── hooks/
│   └── hooks.json            # PostToolUse hook: fires on Edit|Write|MultiEdit
├── skills/
│   └── idiomatic/
│       └── SKILL.md          # Skill: how to read and respond to findings
└── README.md
```

## Files

### `.claude-plugin/plugin.json`
Plugin manifest. Declares the plugin name (`idiomatic`), version, and points to the hooks and skills directories.

### `hooks/hooks.json`
Hook configuration. Registers a PostToolUse hook that matches `Edit|Write|MultiEdit` tools and runs `idio scan --format=claude-hook` with a 30-second timeout. The hook command reads Claude Code's tool event JSON from stdin and writes the response envelope to stdout.

### `skills/idiomatic/SKILL.md`
Skill instructions for Claude. Covers:
- How to read SARIF and compact finding formats
- Fix application workflow (read fix hint → edit file → loop)
- 3-iteration limit before escalating to the user
- Error handling (surface errors, don't work around them)
- Severity semantics (error = must fix, warning = advisory, info = informational)

## Prerequisites

The `idio` CLI must be installed and on PATH. The plugin invokes `idio` directly — it does not bundle the binary.

## Installation

Install the plugin in Claude Code by running `/install-plugin` and providing the path to the `plugin/` directory, or add it to your project's `.claude/plugins.json`:

```json
{
  "plugins": [
    "/path/to/idiomatic/plugin"
  ]
}
```

## Configuration

Create a `.idiomatic.yaml` in your project root with `kind: ProjectConfig`. This declares which capabilities and rule packs to use:

```yaml
apiVersion: rules.idiomatic.dev/v1alpha1
kind: ProjectConfig

capabilities:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - capabilities/semgrep.yaml

packs:
  - repo: https://github.com/adamgilman/idiomatic
    path:
      - packs/go-starter.yaml
```

The hook discovers this file by walking up from the edited file's directory.
