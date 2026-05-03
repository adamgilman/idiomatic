// SPDX-License-Identifier: Apache-2.0

// loader.go — Resolves capabilities and packs from a ProjectConfig into a LoadedProject.
//
//   - When a source has a repo URL, CloneRepo fetches it and paths resolve within the clone
//   - When repo is omitted, paths resolve relative to the config file (local development)
//   - Each unique repo is cloned once (deduped by URL)
//   - Capabilities go into a declarative.Registry; packs are parsed into manifest.Rules
package config

import (
	"fmt"
	"path/filepath"

	"github.com/adamgilman/idiomatic/internal/declarative"
	"github.com/adamgilman/idiomatic/manifest"
)

type LoadedProject struct {
	Registry *declarative.Registry
	Rules    []manifest.Rule
	Warnings []string
}

func LoadProject(cfg *ProjectConfig) (*LoadedProject, error) {
	repoCache := map[string]string{}
	allSources := append(cfg.Capabilities, cfg.Packs...)
	for _, src := range allSources {
		if src.Repo == "" {
			continue
		}
		if _, ok := repoCache[src.Repo]; !ok {
			local, err := declarative.CloneRepo(src.Repo)
			if err != nil {
				return nil, fmt.Errorf("clone %s: %w", src.Repo, err)
			}
			repoCache[src.Repo] = local
		}
	}

	registry := declarative.NewRegistry()
	for _, src := range cfg.Capabilities {
		base := resolveBase(src, repoCache, cfg.Dir)
		for _, p := range src.Path {
			capPath := resolvePath(base, p)
			caps, err := declarative.LoadPath(capPath)
			if err != nil {
				return nil, fmt.Errorf("load capability %s: %w", p, err)
			}
			for _, c := range caps {
				if err := registry.Add(c); err != nil {
					return nil, err
				}
			}
		}
	}

	var allRules []manifest.Rule
	var allWarnings []string
	for _, src := range cfg.Packs {
		base := resolveBase(src, repoCache, cfg.Dir)
		for _, p := range src.Path {
			packPath := resolvePath(base, p)
			result, err := manifest.LoadFile(packPath)
			if err != nil {
				return nil, fmt.Errorf("load pack %s: %w", p, err)
			}
			if len(result.Errors) > 0 {
				return nil, fmt.Errorf("pack %s: %s", p, result.Errors.Error())
			}
			for _, r := range result.Rules {
				allRules = append(allRules, *r)
			}
			allWarnings = append(allWarnings, result.Warnings...)
		}
	}

	return &LoadedProject{
		Registry: registry,
		Rules:    allRules,
		Warnings: allWarnings,
	}, nil
}

func resolveBase(src GitSource, repoCache map[string]string, configDir string) string {
	if src.Repo == "" {
		return configDir
	}
	return repoCache[src.Repo]
}

// resolvePath resolves a single path entry against the source's base.
// Absolute paths are honored as-is; relative paths are joined with the
// base. Without this, filepath.Join silently concatenates absolute paths
// onto the base too — turning "/abs/x.yaml" with base "/tmp/foo" into
// "/tmp/foo/abs/x.yaml" instead of "/abs/x.yaml".
func resolvePath(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}
