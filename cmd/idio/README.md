# cmd/idio — The `idio` CLI

The primary interface for users. Structured command hierarchy using Cobra.

## Commands

```
idio
├── scan          # Run analysis against files
│   --config      # Path to .idiomatic.yaml (default: walk up from CWD)
│   --format      # human (default), sarif, claude-hook
├── manifest
│   ├── validate  # Validate a manifest file or directory
│   └── list      # List rules in a manifest
└── version       # Print version info (injected via ldflags)
```

Hidden (placeholder for future use): `auth`, `config`.

## Files

### `main.go`
Entry point. Calls `cmd.NewCmdRoot().Execute()`.

### `cmd/root.go`
Root command. Registers all subcommands.

### `cmd/scan.go`
The most important command. Three modes based on `--format`:
- **human** (default): Discovers `.idiomatic.yaml`, loads capabilities and packs from git, scans file args, prints `file:line:col: severity: message [rule-id]`.
- **sarif**: Same flow, outputs SARIF 2.1.0 JSON.
- **claude-hook**: Reads PostToolUse hook JSON from stdin, discovers `.idiomatic.yaml` automatically, runs analysis, writes hook response envelope to stdout. Used by the Claude Code plugin.

### `cmd/manifest.go`
`validate` and `list` subcommands for working with rule manifests without running analysis.

### `cmd/auth.go`
Placeholder for future authentication. Hidden from `--help`.

### `cmd/config.go`
Placeholder for future configuration management. Hidden from `--help`.

### `cmd/version.go`
Version output. `Version`, `Commit`, and `Date` set via ldflags at build time.

## Build

```bash
# Development
go build -o dist/idio ./cmd/idio

# With version info
go build -o dist/idio \
  -ldflags "-X .../cmd/idio/cmd.Version=1.0.0 -X .../cmd/idio/cmd.Commit=$(git rev-parse HEAD)" \
  ./cmd/idio
```

## Test

```bash
go test ./cmd/idio/...
```
