package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		filepath.Join(home, ".config", "plan-bender", "defaults.json"),
		filepath.Join(root, ".plan-bender.json"),
		filepath.Join(root, ".plan-bender.local.json"),
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

	data, err = migrateDeprecatedKeys(data)
	if err != nil {
		return PartialConfig{}, err
	}

	var partial PartialConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&partial); err != nil {
		return PartialConfig{}, fmt.Errorf("parsing JSON: %w", err)
	}

	return partial, nil
}

// migrateDeprecatedKeys rewrites removed config keys in raw JSON before typed unmarshal.
// install_target → hard error (user must fix manually).
// backend: linear → linear.enabled: true (silent migration).
// backend: yaml-fs → dropped (default behavior).
// agents: [seq] → agents: {name: true, ...} (silent migration to map format).
// review_with_user: [seq] → review_with_user: <bool> (silent migration; non-empty → true).
func migrateDeprecatedKeys(data []byte) ([]byte, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return data, nil // let the caller handle parse errors
	}
	if _, ok := raw["install_target"]; ok {
		return nil, fmt.Errorf("install_target is removed — replace with agents:\n  \"claude-code\": true\nin your .plan-bender.json")
	}

	modified := false

	// Migrate old agents array format to map format
	if agentsVal, ok := raw["agents"]; ok {
		if agentsList, ok := agentsVal.([]any); ok {
			agentsMap := make(map[string]any, len(agentsList))
			for _, item := range agentsList {
				if name, ok := item.(string); ok {
					agentsMap[name] = true
				}
			}
			raw["agents"] = agentsMap
			modified = true
		}
	}

	// Migrate old review_with_user []string to bool (any non-empty list → true).
	if rwuVal, ok := raw["review_with_user"]; ok {
		if rwuList, ok := rwuVal.([]any); ok {
			raw["review_with_user"] = len(rwuList) > 0
			modified = true
		}
	}

	backend, hasBackend := raw["backend"]
	if hasBackend {
		if backend == "linear" {
			linear, _ := raw["linear"].(map[string]any)
			if linear == nil {
				linear = make(map[string]any)
			}
			if _, exists := linear["enabled"]; !exists {
				linear["enabled"] = true
				raw["linear"] = linear
			}
		}
		delete(raw, "backend")
		modified = true
	}

	if !modified {
		return data, nil
	}

	out, err := json.Marshal(raw)
	if err != nil {
		return data, nil
	}
	return out, nil
}
