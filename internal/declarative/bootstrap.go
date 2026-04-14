// SPDX-License-Identifier: Apache-2.0

// bootstrap.go — Registry type that holds loaded capabilities and wires them
// into the engine as backends.
//
//   - Registry enforces unique names; duplicate capability names error at Add.
//   - BuildBackends silently skips capabilities whose binary is not found,
//     deferring the error to the routing layer if a manifest actually needs it.
//   - CheckRuleVersions enforces semver constraints (^1, ~1.2, >=1,<2, etc.)
//     before any tool runs, so a stale rule pinned to an old major never
//     silently matches a newer capability.
//   - RouteFile replaces hardcoded extension switches in the claudehook path.
package declarative

import (
	"fmt"

	"github.com/adamgilman/idiomatic/internal/engine"
)

// Registry is a set of declarative capabilities loaded from filesystem paths.
// Capabilities are not bundled into the binary — they live as plain YAML
// files alongside rule packs and are loaded by the same mechanism: the user
// (or auto-discovery) hands the registry one or more paths and it walks
// them.
type Registry struct {
	caps   []*Capability
	byName map[string]*Capability
}

// NewRegistry constructs an empty registry. Use Add to populate.
func NewRegistry() *Registry {
	return &Registry{byName: map[string]*Capability{}}
}

// Add appends one capability to the registry. Returns an error on a duplicate
// name.
func (r *Registry) Add(c *Capability) error {
	if _, exists := r.byName[c.Name()]; exists {
		return fmt.Errorf("duplicate capability %q", c.Name())
	}
	r.caps = append(r.caps, c)
	r.byName[c.Name()] = c
	return nil
}

// All returns the slice of capabilities in load order.
func (r *Registry) All() []*Capability { return r.caps }

// ByName returns the capability registered under the given name.
func (r *Registry) ByName(name string) (*Capability, bool) {
	c, ok := r.byName[name]
	return c, ok
}

// Names returns the loaded capability names sorted by load order.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.caps))
	for _, c := range r.caps {
		out = append(out, c.Name())
	}
	return out
}

// BuildBackends returns a backend map keyed by capability name. Each
// capability whose binary is on PATH (or discoverable via walk-up) is
// registered. Capabilities whose binary is missing are silently skipped —
// the routing layer downstream emits a clear error if a manifest requires
// a missing one.
func (r *Registry) BuildBackends() map[string]engine.Backend {
	backends := make(map[string]engine.Backend, len(r.caps))
	for _, c := range r.caps {
		if _, err := c.Detect(); err != nil {
			continue
		}
		backends[c.Name()] = engine.NewCapabilityBackendAdapter(c, c.Name())
	}
	return backends
}

// RouteFile returns the names of every capability that wants to receive the
// given file. Used by claudehook to replace its hardcoded extension switch.
func (r *Registry) RouteFile(path string) []string {
	var routes []string
	for _, c := range r.caps {
		if c.AppliesToFile(path) {
			routes = append(routes, c.Name())
		}
	}
	return routes
}

// VersionMismatch describes one rule whose capability version constraint is
// not satisfied by the loaded capability. The CLI surfaces these to the user
// before any tool runs so a stale rule cannot silently match against a new
// capability schema.
type VersionMismatch struct {
	RuleID         string
	Capability     string
	Constraint     string
	LoadedVersion  string
}

func (m VersionMismatch) Error() string {
	return fmt.Sprintf("rule %q targets capability %q with version constraint %q, "+
		"but the loaded capability is version %s",
		m.RuleID, m.Capability, m.Constraint, m.LoadedVersion)
}

// CheckRuleVersions iterates a slice of (rule_id, capability_name, constraint)
// tuples and verifies that each rule's constraint is satisfied by the loaded
// capability. Returns the first mismatch (if any) so the caller can fail
// loudly. Empty constraints always pass; missing capabilities are NOT
// validated here (the caller already handles "capability not registered").
func (r *Registry) CheckRuleVersions(rules []RuleVersionRequest) error {
	for _, req := range rules {
		cap, ok := r.byName[req.Capability]
		if !ok {
			continue // not our concern; routing layer reports it
		}
		vc, err := ParseConstraint(req.Constraint)
		if err != nil {
			return fmt.Errorf("rule %q: %w", req.RuleID, err)
		}
		ok, err = vc.Matches(cap.Version())
		if err != nil {
			return fmt.Errorf("rule %q: %w", req.RuleID, err)
		}
		if !ok {
			return VersionMismatch{
				RuleID:        req.RuleID,
				Capability:    req.Capability,
				Constraint:    req.Constraint,
				LoadedVersion: cap.Version(),
			}
		}
	}
	return nil
}

// RuleVersionRequest is the per-rule input to CheckRuleVersions. We accept a
// flat tuple instead of manifest.Rule so the engine doesn't have to import
// the rule type at this layer (and so callers can supply rules from any
// source).
type RuleVersionRequest struct {
	RuleID     string
	Capability string
	Constraint string
}

