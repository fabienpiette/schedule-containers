package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DBPath      string
	LogLevel    string
	WebPort     int
	WebHost     string
	DockerHost  string
	PresetsPath string
	Timezone    string
}

func Load() (*Config, error) {
	cfg := &Config{
		DBPath:      envOrDefault("DB_PATH", "/data/schedule-containers.db"),
		LogLevel:    envOrDefault("LOG_LEVEL", "info"),
		WebPort:     8080,
		WebHost:     envOrDefault("WEB_HOST", "0.0.0.0"),
		DockerHost:  envOrDefault("DOCKER_HOST", "unix:///var/run/docker.sock"),
		PresetsPath: envOrDefault("PRESETS_PATH", ""),
		Timezone:    envOrDefault("TZ", "UTC"),
	}

	if v := os.Getenv("WEB_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid WEB_PORT: %w", err)
		}
		cfg.WebPort = p
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}