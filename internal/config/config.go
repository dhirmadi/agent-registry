package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL            string
	Port                   string
	LogLevel               string
	ExternalURL            string
	SessionSecret          string
	CredentialEncryptionKey string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	OTELExporterEndpoint   string
	OTELServiceName        string
	WebhookTimeoutS        int
	WebhookRetries         int
	WebhookWorkers         int
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	return LoadFrom(nil)
}

// LoadFrom reads configuration from the provided map, falling back to os.Getenv
// for missing keys. If env is nil, all values come from os.Getenv.
func LoadFrom(env map[string]string) (*Config, error) {
	get := func(key string) string {
		if env != nil {
			return env[key]
		}
		return os.Getenv(key)
	}

	cfg := &Config{}

	// Required
	cfg.DatabaseURL = get("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("required environment variable DATABASE_URL is not set")
	}
	cfg.SessionSecret = get("SESSION_SECRET")
	if cfg.SessionSecret == "" {
		return nil, fmt.Errorf("required environment variable SESSION_SECRET is not set")
	}
	cfg.CredentialEncryptionKey = get("CREDENTIAL_ENCRYPTION_KEY")
	if cfg.CredentialEncryptionKey == "" {
		return nil, fmt.Errorf("required environment variable CREDENTIAL_ENCRYPTION_KEY is not set")
	}

	// Strings with defaults
	cfg.Port = getOrDefault(get, "PORT", "8090")
	cfg.LogLevel = getOrDefault(get, "LOG_LEVEL", "info")
	cfg.ExternalURL = getOrDefault(get, "EXTERNAL_URL", "http://localhost:8090")
	cfg.OTELServiceName = getOrDefault(get, "OTEL_SERVICE_NAME", "agentic-registry")

	// Optional strings
	cfg.GoogleOAuthClientID = get("GOOGLE_OAUTH_CLIENT_ID")
	cfg.GoogleOAuthClientSecret = get("GOOGLE_OAUTH_CLIENT_SECRET")
	cfg.OTELExporterEndpoint = get("OTEL_EXPORTER_OTLP_ENDPOINT")

	// Ints with defaults
	var err error
	cfg.WebhookTimeoutS, err = getIntOrDefault(get, "WEBHOOK_TIMEOUT", 5)
	if err != nil {
		return nil, err
	}
	cfg.WebhookRetries, err = getIntOrDefault(get, "WEBHOOK_RETRIES", 3)
	if err != nil {
		return nil, err
	}
	cfg.WebhookWorkers, err = getIntOrDefault(get, "WEBHOOK_WORKERS", 4)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func getOrDefault(get func(string) string, key, defaultVal string) string {
	if v := get(key); v != "" {
		return v
	}
	return defaultVal
}

func getIntOrDefault(get func(string) string, key string, defaultVal int) (int, error) {
	v := get(key)
	if v == "" {
		return defaultVal, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %w", key, err)
	}
	return n, nil
}
