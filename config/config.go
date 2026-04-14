// SPDX-License-Identifier: Apache-2.0

// config.go — ProjectConfig types and YAML parser for .idiomatic.yaml files.
//
//   - ProjectConfig declares git-hosted capability and pack sources (repo + paths)
//   - Load performs strict validation: Kind must be "ProjectConfig", and both
//     capabilities and packs must be non-empty with repo and path on every entry
//   - This is pure parsing/validation; actual git cloning happens in loader.go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ProjectConfig struct {
	APIVersion   string      `yaml:"apiVersion"`
	Kind         string      `yaml:"kind"`
	Capabilities []GitSource `yaml:"capabilities"`
	Packs        []GitSource `yaml:"packs"`
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
		if src.Repo == "" {
			return nil, fmt.Errorf("%s: capabilities[%d].repo is required", path, i)
		}
		if len(src.Path) == 0 {
			return nil, fmt.Errorf("%s: capabilities[%d].path is required", path, i)
		}
	}
	for i, src := range cfg.Packs {
		if src.Repo == "" {
			return nil, fmt.Errorf("%s: packs[%d].repo is required", path, i)
		}
		if len(src.Path) == 0 {
			return nil, fmt.Errorf("%s: packs[%d].path is required", path, i)
		}
	}
	return &cfg, nil
}
