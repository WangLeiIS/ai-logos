package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// MinIOConfig holds MinIO storage configuration
type MinIOConfig struct {
	Endpoint  string `yaml:"endpoint"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	Bucket    string `yaml:"bucket"`
	UseSSL    bool   `yaml:"use_ssl"`
}

// OAuthConfig holds OAuth provider configuration
type OAuthConfig struct {
	GithubClientID     string `yaml:"github_client_id"`
	GithubClientSecret string `yaml:"github_client_secret"`
	GoogleClientID     string `yaml:"google_client_id"`
	GoogleClientSecret string `yaml:"google_client_secret"`
	RedirectBase       string `yaml:"redirect_base"`
}

// Config holds the application configuration
type Config struct {
	Listen string      `yaml:"listen"`
	DB     string      `yaml:"db"`
	MinIO  MinIOConfig `yaml:"minio"`
	OAuth  OAuthConfig `yaml:"oauth"`
}

// LoadConfig loads configuration from config.yaml and overrides with environment variables
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Override with environment variables
	if v := os.Getenv("IROLLHUB_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("IROLLHUB_DB"); v != "" {
		cfg.DB = v
	}
	if v := os.Getenv("IROLLHUB_MINIO_ENDPOINT"); v != "" {
		cfg.MinIO.Endpoint = v
	}
	if v := os.Getenv("IROLLHUB_MINIO_ACCESS_KEY"); v != "" {
		cfg.MinIO.AccessKey = v
	}
	if v := os.Getenv("IROLLHUB_MINIO_SECRET_KEY"); v != "" {
		cfg.MinIO.SecretKey = v
	}
	if v := os.Getenv("IROLLHUB_GITHUB_CLIENT_ID"); v != "" {
		cfg.OAuth.GithubClientID = v
	}
	if v := os.Getenv("IROLLHUB_GITHUB_CLIENT_SECRET"); v != "" {
		cfg.OAuth.GithubClientSecret = v
	}
	if v := os.Getenv("IROLLHUB_GOOGLE_CLIENT_ID"); v != "" {
		cfg.OAuth.GoogleClientID = v
	}
	if v := os.Getenv("IROLLHUB_GOOGLE_CLIENT_SECRET"); v != "" {
		cfg.OAuth.GoogleClientSecret = v
	}

	// Trim whitespace from string values
	cfg.Listen = strings.TrimSpace(cfg.Listen)
	cfg.DB = strings.TrimSpace(cfg.DB)
	cfg.MinIO.Endpoint = strings.TrimSpace(cfg.MinIO.Endpoint)
	cfg.MinIO.AccessKey = strings.TrimSpace(cfg.MinIO.AccessKey)
	cfg.MinIO.SecretKey = strings.TrimSpace(cfg.MinIO.SecretKey)
	cfg.MinIO.Bucket = strings.TrimSpace(cfg.MinIO.Bucket)
	cfg.OAuth.GithubClientID = strings.TrimSpace(cfg.OAuth.GithubClientID)
	cfg.OAuth.GithubClientSecret = strings.TrimSpace(cfg.OAuth.GithubClientSecret)
	cfg.OAuth.GoogleClientID = strings.TrimSpace(cfg.OAuth.GoogleClientID)
	cfg.OAuth.GoogleClientSecret = strings.TrimSpace(cfg.OAuth.GoogleClientSecret)
	cfg.OAuth.RedirectBase = strings.TrimSpace(cfg.OAuth.RedirectBase)

	return &cfg, nil
}
