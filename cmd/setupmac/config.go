package main

import (
	"encoding/json"
	"os"
)

const configPath = "/etc/setupmac.conf"

// savedConfig holds non-sensitive values persisted between runs so the
// operator does not have to retype them each time. Passwords are never saved.
type savedConfig struct {
	Username       string `json:"username,omitempty"`
	GitHubKeysUser string `json:"githubKeysUser,omitempty"`
	BannerOrg      string `json:"bannerOrg,omitempty"`
}

// loadConfig reads the saved config file. Returns an empty struct if the file
// does not exist or cannot be parsed — the caller treats all fields as unset.
func loadConfig() savedConfig {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return savedConfig{}
	}
	var cfg savedConfig
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// saveConfig writes cfg to the config file as JSON. Errors are silently
// ignored — a failed save means the operator is prompted again next run,
// which is inconvenient but not a setup failure.
func saveConfig(cfg savedConfig) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath, data, 0600)
}
