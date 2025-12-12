package config

import (
	"fmt"
	"os"
)

// Config holds all configuration for the MCP sandbox server
type Config struct {
	HTTPAddr        string
	APIToken        string
	SandboxRoot     string // Path where server reads/writes files (filesystem operations)
	SandboxHostPath string // Path on Docker host for bind mounts (Docker operations) - may be same as SandboxRoot
	FileSecret      string
	PublicBaseURL   string
	DockerHost      string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	sandboxRoot := os.Getenv("SANDBOX_ROOT")

	cfg := &Config{
		HTTPAddr:        getEnvOrDefault("MCP_HTTP_ADDR", ":8080"),
		APIToken:        os.Getenv("MCP_API_TOKEN"),
		SandboxRoot:     sandboxRoot,
		SandboxHostPath: getEnvOrDefault("SANDBOX_HOST_PATH", sandboxRoot), // Default to SandboxRoot if not set
		FileSecret:      os.Getenv("FILE_SECRET"),
		PublicBaseURL:   os.Getenv("PUBLIC_BASE_URL"),
		DockerHost:      os.Getenv("DOCKER_HOST"),
	}

	// Validate required fields
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("MCP_API_TOKEN is required")
	}
	if cfg.SandboxRoot == "" {
		return nil, fmt.Errorf("SANDBOX_ROOT is required")
	}
	if cfg.FileSecret == "" {
		return nil, fmt.Errorf("FILE_SECRET is required")
	}
	if cfg.PublicBaseURL == "" {
		return nil, fmt.Errorf("PUBLIC_BASE_URL is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
