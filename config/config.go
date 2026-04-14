// SPDX-License-Identifier: Apache-2.0

// config.go — ProjectConfig types and YAML parser for .idiomatic.yaml files.
//
//   - ProjectConfig declares capability and pack sources as repo + paths entries
//   - When repo is set, paths are resolved from a git clone of that repo
//   - When repo is omitted, paths are resolved relative to the config file (local dev)
//   - Load performs strict validation: Kind must be "ProjectConfig", capabilities
//     and packs must be non-empty, every entry must have at least one path
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	APIVersion   string      `yaml:"apiVersion"`
	Kind         string      `yaml:"kind"`
	Capabilities []GitSource `yaml:"capabilities"`
	Packs        []GitSource `yaml:"packs"`

	// Dir is the directory containing the config file. Used to resolve
	// local paths when repo is omitted. Set by Load(), not from YAML.
	Dir string `yaml:"-"`
}

type GitSource struct {
	Repo string   `yaml:"repo"`
	Path []string `yaml:"path"`
}

func Load(path string) (*ProjectConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cfg.Kind != "ProjectConfig" {
		return nil, fmt.Errorf("%s: expected kind ProjectConfig, got %q", path, cfg.Kind)
	}
	if len(cfg.Capabilities) == 0 {
		return nil, fmt.Errorf("%s: no capabilities declared", path)
	}
	if len(cfg.Packs) == 0 {
		return nil, fmt.Errorf("%s: no packs declared", path)
	}
	for i, src := range cfg.Capabilities {
		if len(src.Path) == 0 {
			return nil, fmt.Errorf("%s: capabilities[%d].path is required", path, i)
		}
	}
	for i, src := range cfg.Packs {
		if len(src.Path) == 0 {
			return nil, fmt.Errorf("%s: packs[%d].path is required", path, i)
		}
	}
	cfg.Dir = filepath.Dir(path)
	return &cfg, nil
}
