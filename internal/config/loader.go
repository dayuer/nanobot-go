package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// NanobotHome returns the nanobot home directory (~/.nanobot).
func NanobotHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nanobot")
}

// GetConfigPath returns the config file path.
// Uses nginx-style layout: ~/.nanobot/conf/config.json
// Falls back to legacy ~/.nanobot/config.json if it exists.
func GetConfigPath() string {
	root := NanobotHome()
	newPath := filepath.Join(root, "conf", "config.json")

	// If new path exists, use it
	if _, err := os.Stat(newPath); err == nil {
		return newPath
	}

	// If legacy path exists, migrate it
	legacyPath := filepath.Join(root, "config.json")
	if _, err := os.Stat(legacyPath); err == nil {
		// Auto-migrate: move to conf/
		os.MkdirAll(filepath.Join(root, "conf"), 0755)
		if err := os.Rename(legacyPath, newPath); err == nil {
			return newPath
		}
		return legacyPath // migration failed, use legacy
	}

	return newPath // default to new path
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
