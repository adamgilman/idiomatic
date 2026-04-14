# Contributing to Idiomatic

Thank you for your interest in contributing. This guide covers everything you need to get started.

## Prerequisites

- Go 1.25+
- make

For running rule packs locally, you'll also need the external tools referenced by the capabilities you're testing (e.g. `semgrep`, `eslint`, `golangci-lint`).

## Building

```bash
make build        # Build the idio binary to dist/idio
make install      # Build and install to ~/.local/bin/idio
```

## Testing

```bash
make test         # Run all unit tests
make vet          # Run go vet
make lint         # Run golangci-lint (requires golangci-lint on PATH)
```

## Development Workflow

1. Fork the repository
2. Create a feature branch from `main`
3. Make your changes
4. Run `make test` and `make vet` to verify
5. Commit with a clear message following [Conventional Commits](https://www.conventionalcommits.org/)
6. Open a pull request against `main`

## Adding Rule Packs

Rule packs are pure YAML. No Go code changes are required.

1. Create a new YAML file in `packs/` (copy an existing pack as a template)
2. Follow the format in [`docs/spec/rule-pack.md`](docs/spec/rule-pack.md)
3. Run `idio manifest validate packs/your-pack.yaml` to check syntax
4. Create a test `.idiomatic.yaml` referencing your pack and run `idio scan <test-dir>` to verify end-to-end
5. Run `make test` to ensure manifest validation tests still pass

See [`docs/authoring/rule-pack.md`](docs/authoring/rule-pack.md) for a full tutorial.

## Adding Capabilities

Capabilities are YAML wrappers around external CLI tools. No Go code changes are required.

1. Create a new YAML file in `capabilities/` (copy an existing capability as a template)
2. Follow the format in [`docs/spec/capability.md`](docs/spec/capability.md)
3. Reference the new capability file in your project's `.idiomatic.yaml` under the `capabilities:` section
4. Run `make test` to verify

See [`docs/authoring/capability.md`](docs/authoring/capability.md) for a full tutorial.

## Code Style

- All Go code must pass `gofmt` and `go vet`
- All new `.go` files must include the SPDX header: `// SPDX-License-Identifier: Apache-2.0`
- Follow existing patterns in the codebase

## What Not to Do

- Don't embed analysis logic in Go. Write a YAML capability that wraps an external tool.
- Don't add Go packages for capabilities. Every tool integration is YAML.
- Don't bundle capability or pack YAMLs into the binary. They live on the filesystem.

## Reporting Issues

Use [GitHub Issues](https://github.com/adamgilman/idiomatic/issues) for bug reports and feature requests.

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
