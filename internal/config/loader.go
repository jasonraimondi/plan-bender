package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads config from 3 layers (global, project, local) and merges them over defaults.
func Load(root string) (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	return loadWithHome(root, home)
}

func loadWithHome(root, home string) (Config, error) {
	base := Defaults()

	paths := []string{
		filepath.Join(home, ".config", "plan-bender", "defaults.yaml"),
		filepath.Join(root, ".plan-bender.yaml"),
		filepath.Join(root, ".plan-bender.local.yaml"),
	}

	for _, p := range paths {
		layer, err := readPartial(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, fmt.Errorf("loading %s: %w", filepath.Base(p), err)
		}
		base = merge(base, layer)
	}

	expandEnv(&base)

	if err := validate(&base); err != nil {
		return Config{}, err
	}

	return base, nil
}

func readPartial(path string) (PartialConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PartialConfig{}, err
	}

	if err := checkDeprecatedKeys(data); err != nil {
		return PartialConfig{}, err
	}

	var partial PartialConfig
	if err := yaml.Unmarshal(data, &partial); err != nil {
		return PartialConfig{}, fmt.Errorf("parsing YAML: %w", err)
	}

	return partial, nil
}

func checkDeprecatedKeys(data []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil // let the caller handle parse errors
	}
	if _, ok := raw["install_target"]; ok {
		return fmt.Errorf("install_target is removed — replace with agents: [claude-code] in your .plan-bender.yaml")
	}
	return nil
}
