package config

import "os"

// expandEnv resolves environment variable references in config values.
// Supports $VAR and ${VAR} syntax. Only applied to fields that commonly hold secrets.
func expandEnv(cfg *Config) {
	cfg.Linear.APIKey = os.ExpandEnv(cfg.Linear.APIKey)
	cfg.Linear.Team = os.ExpandEnv(cfg.Linear.Team)
}
