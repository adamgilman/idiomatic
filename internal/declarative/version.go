// SPDX-License-Identifier: Apache-2.0

// version.go — Semver constraint parsing and matching, wrapping
// Masterminds/semver/v3.
//
//   - The empty constraint (no version: field on a rule) matches every version,
//     so rules that don't pin always pass.
//   - Constraint syntax supports everything Masterminds/semver provides: exact,
//     caret (^1), tilde (~1.2), range (>=1,<2), and wildcard (1.x).
//   - Constraints are validated at manifest load time so typos surface early
//     rather than at runtime when the tool would have already run.
package declarative

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// parseSemver wraps semver.NewVersion with a more idiomatic function name.
// Returns a parsed version or an error if the string isn't a valid semver.
func parseSemver(s string) (*semver.Version, error) {
	return semver.NewVersion(s)
}

// VersionConstraint is a parsed semver constraint expression. The empty
// constraint matches every version (used when a rule doesn't pin a version).
type VersionConstraint struct {
	raw string
	c   *semver.Constraints
}

// ParseConstraint parses a semver constraint string. Accepted forms include:
//
//   ""             — no constraint, matches anything
//   "1.0.0"        — exact version
//   "^1.2"         — caret: >=1.2.0, <2.0.0
//   "~1.2"         — tilde: >=1.2.0, <1.3.0
//   ">=1.2, <2"    — explicit range
//   "1.x"          — wildcard
//
// Returns a VersionConstraint that can be tested with Matches.
func ParseConstraint(s string) (VersionConstraint, error) {
	if s == "" {
		return VersionConstraint{raw: ""}, nil
	}
	c, err := semver.NewConstraint(s)
	if err != nil {
		return VersionConstraint{}, fmt.Errorf("invalid version constraint %q: %w", s, err)
	}
	return VersionConstraint{raw: s, c: c}, nil
}

// Matches reports whether the given version satisfies this constraint. The
// empty constraint always matches.
func (vc VersionConstraint) Matches(version string) (bool, error) {
	if vc.c == nil {
		return true, nil
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, fmt.Errorf("capability version %q is not valid semver: %w", version, err)
	}
	return vc.c.Check(v), nil
}

// IsEmpty reports whether this constraint matches anything (i.e. the rule
// did not declare a version constraint).
func (vc VersionConstraint) IsEmpty() bool { return vc.c == nil }

// String returns the original constraint string ("" for the empty constraint).
func (vc VersionConstraint) String() string { return vc.raw }
