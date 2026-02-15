package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// GetConfigPath returns the default config file path (~/.nanobot/config.json).
func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nanobot", "config.json")
}

// Load reads configuration from a JSON file.
// If path is empty, uses the default config path.
// If the file doesn't exist, returns DefaultConfig().
func Load(path string) (Config, error) {
	if path == "" {
		path = GetConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, err
	}

	cfg := DefaultConfig() // start with defaults so zero-value fields get filled
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), err
	}
	return cfg, nil
}

// Save writes configuration to a JSON file.
// If path is empty, uses the default config path.
func Save(cfg Config, path string) error {
	if path == "" {
		path = GetConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
