package config

import "os"

func expandEnv(cfg *Config) {
	cfg.Linear.APIKey = os.ExpandEnv(cfg.Linear.APIKey)
	cfg.Linear.Team = os.ExpandEnv(cfg.Linear.Team)
}
