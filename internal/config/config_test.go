package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	os.Clearenv()
	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DBPath != "/data/schedule-containers.db" {
		t.Errorf("expected default DBPath, got %s", cfg.DBPath)
	}
	if cfg.WebPort != 8080 {
		t.Errorf("expected default WebPort 8080, got %d", cfg.WebPort)
	}
	if cfg.WebHost != "0.0.0.0" {
		t.Errorf("expected default WebHost 0.0.0.0, got %s", cfg.WebHost)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default LogLevel info, got %s", cfg.LogLevel)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Clearenv()
	os.Setenv("DB_PATH", "/tmp/test.db")
	os.Setenv("WEB_PORT", "9090")
	os.Setenv("WEB_HOST", "127.0.0.1")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("DOCKER_HOST", "unix:///var/run/docker.sock")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.DBPath != "/tmp/test.db" {
		t.Errorf("expected /tmp/test.db, got %s", cfg.DBPath)
	}
	if cfg.WebPort != 9090 {
		t.Errorf("expected 9090, got %d", cfg.WebPort)
	}
	if cfg.WebHost != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", cfg.WebHost)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected debug, got %s", cfg.LogLevel)
	}
	if cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("expected docker socket, got %s", cfg.DockerHost)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	os.Clearenv()
	os.Setenv("WEB_PORT", "not-a-number")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}
