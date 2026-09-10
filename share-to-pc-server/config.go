package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type config struct {
	Port        string `json:"port"`
	MpvPath     string `json:"mpvPath,omitempty"`
	BrowserPath string `json:"browserPath,omitempty"`
}

func configPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(filepath.Dir(exe), "config.json")
}

func defaultConfig() config {
	return config{Port: "8888"}
}

func loadConfig() config {
	cfg := defaultConfig()
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Port == "" {
		return defaultConfig()
	}
	if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		return defaultConfig()
	}
	return cfg
}

func saveConfig(cfg config) error {
	existing := loadConfig()
	if cfg.Port != "" {
		existing.Port = cfg.Port
	}
	if cfg.MpvPath != "" {
		existing.MpvPath = cfg.MpvPath
	}
	if cfg.BrowserPath != "" {
		existing.BrowserPath = cfg.BrowserPath
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(configPath(), data, 0644)
}
