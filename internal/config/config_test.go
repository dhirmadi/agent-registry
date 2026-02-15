package config

import (
	"testing"
)

func TestLoad_RequiredVars(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
	}{
		{
			name:    "missing DATABASE_URL",
			env:     map[string]string{"SESSION_SECRET": "abc", "CREDENTIAL_ENCRYPTION_KEY": "def"},
			wantErr: "DATABASE_URL",
		},
		{
			name:    "missing SESSION_SECRET",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/test", "CREDENTIAL_ENCRYPTION_KEY": "def"},
			wantErr: "SESSION_SECRET",
		},
		{
			name:    "missing CREDENTIAL_ENCRYPTION_KEY",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/test", "SESSION_SECRET": "abc"},
			wantErr: "CREDENTIAL_ENCRYPTION_KEY",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadFrom(tc.env)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !contains(got, tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", got, tc.wantErr)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":              "postgres://localhost/test",
		"SESSION_SECRET":            "abc123",
		"CREDENTIAL_ENCRYPTION_KEY": "12345678901234567890123456789012",
	}

	cfg, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"Port", cfg.Port, "8090"},
		{"LogLevel", cfg.LogLevel, "info"},
		{"ExternalURL", cfg.ExternalURL, "http://localhost:8090"},
		{"OTELServiceName", cfg.OTELServiceName, "agentic-registry"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}

	intTests := []struct {
		name string
		got  int
		want int
	}{
		{"WebhookTimeoutS", cfg.WebhookTimeoutS, 5},
		{"WebhookRetries", cfg.WebhookRetries, 3},
		{"WebhookWorkers", cfg.WebhookWorkers, 4},
	}

	for _, tc := range intTests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %d, want %d", tc.got, tc.want)
			}
		})
	}
}

func TestLoad_CustomValues(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":                "postgres://custom:5432/mydb",
		"SESSION_SECRET":              "my-secret",
		"CREDENTIAL_ENCRYPTION_KEY":   "my-key-1234567890123456789012345",
		"PORT":                        "9090",
		"LOG_LEVEL":                   "debug",
		"EXTERNAL_URL":                "https://registry.example.com",
		"GOOGLE_OAUTH_CLIENT_ID":      "google-id",
		"GOOGLE_OAUTH_CLIENT_SECRET":  "google-secret",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "localhost:4317",
		"OTEL_SERVICE_NAME":           "my-registry",
		"WEBHOOK_TIMEOUT":             "10",
		"WEBHOOK_RETRIES":             "5",
		"WEBHOOK_WORKERS":             "8",
	}

	cfg, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.DatabaseURL != "postgres://custom:5432/mydb" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q", cfg.LogLevel)
	}
	if cfg.ExternalURL != "https://registry.example.com" {
		t.Errorf("ExternalURL = %q", cfg.ExternalURL)
	}
	if cfg.GoogleOAuthClientID != "google-id" {
		t.Errorf("GoogleOAuthClientID = %q", cfg.GoogleOAuthClientID)
	}
	if cfg.GoogleOAuthClientSecret != "google-secret" {
		t.Errorf("GoogleOAuthClientSecret = %q", cfg.GoogleOAuthClientSecret)
	}
	if cfg.OTELExporterEndpoint != "localhost:4317" {
		t.Errorf("OTELExporterEndpoint = %q", cfg.OTELExporterEndpoint)
	}
	if cfg.OTELServiceName != "my-registry" {
		t.Errorf("OTELServiceName = %q", cfg.OTELServiceName)
	}
	if cfg.WebhookTimeoutS != 10 {
		t.Errorf("WebhookTimeoutS = %d", cfg.WebhookTimeoutS)
	}
	if cfg.WebhookRetries != 5 {
		t.Errorf("WebhookRetries = %d", cfg.WebhookRetries)
	}
	if cfg.WebhookWorkers != 8 {
		t.Errorf("WebhookWorkers = %d", cfg.WebhookWorkers)
	}
}

func TestLoad_EncryptionKeyLength(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name:    "valid 32-byte key",
			key:     "12345678901234567890123456789012",
			wantErr: false,
		},
		{
			name:    "too short key (16 bytes)",
			key:     "1234567890123456",
			wantErr: true,
		},
		{
			name:    "too long key (64 bytes)",
			key:     "1234567890123456789012345678901234567890123456789012345678901234",
			wantErr: true,
		},
		{
			name:    "3-byte key",
			key:     "def",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{
				"DATABASE_URL":              "postgres://localhost/test",
				"SESSION_SECRET":            "abc123",
				"CREDENTIAL_ENCRYPTION_KEY": tc.key,
			}
			_, err := LoadFrom(env)
			if tc.wantErr && err == nil {
				t.Fatal("expected error for invalid key length, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error for valid key, got %v", err)
			}
			if tc.wantErr && err != nil && !contains(err.Error(), "32 bytes") {
				t.Errorf("expected error to mention 32 bytes, got %q", err.Error())
			}
		})
	}
}

func TestLoad_InvalidIntValues(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":              "postgres://localhost/test",
		"SESSION_SECRET":            "abc",
		"CREDENTIAL_ENCRYPTION_KEY": "12345678901234567890123456789012",
		"WEBHOOK_TIMEOUT":           "not-a-number",
	}

	_, err := LoadFrom(env)
	if err == nil {
		t.Fatal("expected error for invalid int, got nil")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLoad_MCPEnabled_Default(t *testing.T) {
	env := map[string]string{
		"DATABASE_URL":              "postgres://localhost/test",
		"SESSION_SECRET":            "abc123",
		"CREDENTIAL_ENCRYPTION_KEY": "12345678901234567890123456789012",
	}

	cfg, err := LoadFrom(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.MCPEnabled {
		t.Error("MCPEnabled should default to true")
	}
}

func TestLoad_MCPEnabled(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":              "postgres://localhost/test",
		"SESSION_SECRET":            "abc123",
		"CREDENTIAL_ENCRYPTION_KEY": "12345678901234567890123456789012",
	}

	tests := []struct {
		name string
		val  string
		want bool
	}{
		{"default (empty)", "", true},
		{"explicit true", "true", true},
		{"explicit false", "false", false},
		{"1 is true", "1", true},
		{"0 is false", "0", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := make(map[string]string)
			for k, v := range base {
				env[k] = v
			}
			if tc.val != "" {
				env["MCP_ENABLED"] = tc.val
			}
			cfg, err := LoadFrom(env)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.MCPEnabled != tc.want {
				t.Errorf("MCPEnabled = %v, want %v", cfg.MCPEnabled, tc.want)
			}
		})
	}
}
