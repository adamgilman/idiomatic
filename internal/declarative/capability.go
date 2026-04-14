// SPDX-License-Identifier: Apache-2.0

// capability.go — The Capability implementation that satisfies engine.Capability
// and dispatches analysis to list-mode or scalar-mode signal handlers.
//
//   - Analyze is the main entry point. It dispatches on signal.shape: "list"
//     delegates to analyzeList (batch or per-file), "scalar" to analyzeScalar
//     (per-rule). This is the central fork in the entire runtime.
//   - analyzeScalar lives in signal_scalar.go, not here; analyzeList calls
//     runBatch/runPerFile from run.go, then parseListSignal from signal_list.go.
//   - The compile-time interface check (var _ engine.Capability) at the bottom
//     ensures this type stays in sync with the engine contract.
package declarative

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adamgilman/idiomatic/internal/engine"
)

// Capability is a YAML-driven engine.Capability. One CapabilitySpec instance
// becomes one Capability; the same Capability serves many rules (per:batch
// invocations) or one invocation per rule (per:rule).
type Capability struct {
	spec CapabilitySpec
}

// Spec returns the parsed spec. Used by tests and registration glue.
func (c *Capability) Spec() CapabilitySpec { return c.spec }

// Name returns the routing key (rules reference this via detector.capability).
func (c *Capability) Name() string { return c.spec.Metadata.Name }

// Version returns the capability's semver string from metadata.version.
// Rules can pin to a major or range via detector.version.
func (c *Capability) Version() string { return c.spec.Metadata.Version }

// AppliesToFile reports whether this capability cares about the given file
// path. Used by claudehook for routing.
func (c *Capability) AppliesToFile(path string) bool {
	if len(c.spec.Spec.AppliesToFiles.Extensions) == 0 {
		return true
	}
	for _, ext := range c.spec.Spec.AppliesToFiles.Extensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// ValidateConfig is the engine.Capability hook called once per rule at
// manifest load time.
func (c *Capability) ValidateConfig(config map[string]any) error {
	return validateInputs(c.spec.Spec.Inputs, config)
}

// Detect locates the underlying binary using either the spec's discover
// strategy (walk_up_from_cwd) or, by default, exec.LookPath.
func (c *Capability) Detect() (*engine.ToolInfo, error) {
	cwd, _ := os.Getwd()
	bin, _, err := c.discoverBinary(cwd)
	if err != nil {
		return nil, err
	}
	return &engine.ToolInfo{Name: c.spec.Spec.Requires.Binary, Path: bin}, nil
}

// Analyze runs the capability against the request. Dispatch is by signal.shape:
// list-mode runs the tool once (per:batch) and walks the array of results;
// scalar-mode runs the tool once per rule and queries the result with
// fire_when expressions.
func (c *Capability) Analyze(ctx context.Context, req engine.AnalysisRequest) ([]engine.Finding, error) {
	if len(req.Rules) == 0 {
		return nil, nil
	}
	if len(req.Files) == 0 && len(c.spec.Spec.AppliesToFiles.Extensions) > 0 {
		return nil, nil
	}

	switch c.spec.Spec.Signal.Shape {
	case "list":
		return c.analyzeList(ctx, req)
	case "scalar":
		return c.analyzeScalar(ctx, req)
	default:
		return nil, fmt.Errorf("capability %q: unsupported signal.shape %q", c.Name(), c.spec.Spec.Signal.Shape)
	}
}

// analyzeList runs list-mode capabilities. The dispatch picks per:batch
// (single invocation) or per:file (one invocation per target file) and
// concatenates the findings.
func (c *Capability) analyzeList(ctx context.Context, req engine.AnalysisRequest) ([]engine.Finding, error) {
	switch c.spec.Spec.Run.Per {
	case "batch":
		out, err := c.runBatch(ctx, req)
		if err != nil {
			return nil, err
		}
		if out == nil {
			return nil, nil
		}
		return c.parseListSignal(out, req.Rules)
	case "file":
		outs, err := c.runPerFile(ctx, req)
		if err != nil {
			return nil, err
		}
		var all []engine.Finding
		for _, out := range outs {
			findings, err := c.parseListSignal(out, req.Rules)
			if err != nil {
				return nil, err
			}
			all = append(all, findings...)
		}
		return all, nil
	default:
		return nil, fmt.Errorf("capability %q: list signal requires run.per=batch|file (got %q)", c.Name(), c.spec.Spec.Run.Per)
	}
}

// Verify Capability satisfies engine.Capability at compile time.
var _ engine.Capability = (*Capability)(nil)
