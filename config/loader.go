// SPDX-License-Identifier: Apache-2.0

// loader.go — Clones git repos and loads capabilities + packs into a LoadedProject.
//
//   - LoadProject clones each unique repo once (deduped by URL) then resolves paths within clones
//   - Capabilities are loaded into a declarative.Registry; packs are parsed into manifest.Rules
//   - Pack load errors are fatal; pack warnings are collected and returned for the caller to surface
//   - This is the bridge between the config YAML and the engine's runtime types
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
	for _, src := range cfg.Capabilities {
		if _, ok := repoCache[src.Repo]; !ok {
			local, err := declarative.CloneRepo(src.Repo)
			if err != nil {
				return nil, fmt.Errorf("clone %s: %w", src.Repo, err)
			}
			repoCache[src.Repo] = local
		}
	}
	for _, src := range cfg.Packs {
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
		base := repoCache[src.Repo]
		for _, p := range src.Path {
			capPath := filepath.Join(base, p)
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
		base := repoCache[src.Repo]
		for _, p := range src.Path {
			packPath := filepath.Join(base, p)
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
