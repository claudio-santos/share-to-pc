package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
)

type config struct {
	Port string `json:"port"`
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
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(configPath(), data, 0644)
}