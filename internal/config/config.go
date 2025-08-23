package config

import (
	"log"
	"os"
	"path/filepath"
)

type Config struct {
	BotToken    string
	DatabaseURL string
	DefaultTZ   string
}

func Load() Config {
	cfg := Config{
		BotToken:    os.Getenv("BOT_TOKEN"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DefaultTZ:   os.Getenv("DEFAULT_TZ"),
	}
	if cfg.BotToken == "" {
		log.Fatal("BOT_TOKEN is not set")
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "./data/data.db"
	}
	if cfg.DefaultTZ == "" {
		cfg.DefaultTZ = "Europe/Kyiv"
	}
	dir := filepath.Dir(cfg.DatabaseURL)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("failed to create data dir: %v", err)
		}
	}
	return cfg
}
