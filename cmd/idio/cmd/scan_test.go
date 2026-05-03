// SPDX-License-Identifier: Apache-2.0

// scan_test.go — Tests for scan command helpers, focused on regression
// coverage for the cross-cwd hook integration (issue U1).

package cmd

import "testing"

// TestHookEditDir verifies the helper that extracts the edited file's parent
// directory from a PostToolUse hook payload. This is the lookup key used to
// discover the project's .idiomatic.yaml — it must use the file's path, NOT
// idio's own cwd, so the hook works when Claude Code is running in a
// different project than the one being edited.
func TestHookEditDir(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		want  string
	}{
		{
			name:  "edit payload with absolute path",
			stdin: `{"tool_name":"Edit","tool_input":{"file_path":"/home/u/proj/main.go"}}`,
			want:  "/home/u/proj",
		},
		{
			name:  "write payload with nested path",
			stdin: `{"tool_name":"Write","tool_input":{"file_path":"/home/u/proj/internal/foo/bar.go"}}`,
			want:  "/home/u/proj/internal/foo",
		},
		{
			name:  "missing file_path returns empty",
			stdin: `{"tool_name":"Edit","tool_input":{}}`,
			want:  "",
		},
		{
			name:  "malformed json returns empty",
			stdin: `not json`,
			want:  "",
		},
		{
			name:  "empty stdin returns empty",
			stdin: ``,
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hookEditDir([]byte(tt.stdin))
			if got != tt.want {
				t.Errorf("hookEditDir(%q) = %q, want %q", tt.stdin, got, tt.want)
			}
		})
	}
}
