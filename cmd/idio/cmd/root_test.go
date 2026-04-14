// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"testing"
)

func TestRootCommand(t *testing.T) {
	cmd := NewCmdRoot()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("expected help output, got empty string")
	}
}

func TestRootHasSubcommands(t *testing.T) {
	cmd := NewCmdRoot()
	subcommands := cmd.Commands()

	expected := map[string]bool{
		"version": false,
		"config":  false,
		"auth":    false,
	}

	for _, sub := range subcommands {
		if _, ok := expected[sub.Name()]; ok {
			expected[sub.Name()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}
