package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPManifestHandler_GetManifest(t *testing.T) {
	h := NewMCPManifestHandler("https://registry.example.com")

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	w := httptest.NewRecorder()

	h.GetManifest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("expected Cache-Control header, got %s", cc)
	}

	var manifest MCPManifest
	if err := json.NewDecoder(w.Body).Decode(&manifest); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if manifest.Name != "agentic-registry" {
		t.Errorf("expected name agentic-registry, got %s", manifest.Name)
	}
	if manifest.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", manifest.Version)
	}
	if manifest.Description == "" {
		t.Error("expected non-empty description")
	}

	// Verify transport
	transport, ok := manifest.Transport["streamableHttp"].(map[string]interface{})
	if !ok {
		t.Fatal("expected streamableHttp transport")
	}
	url, _ := transport["url"].(string)
	if url != "https://registry.example.com/mcp/v1" {
		t.Errorf("expected transport URL https://registry.example.com/mcp/v1, got %s", url)
	}

	// Verify authentication
	authType, _ := manifest.Authentication["type"].(string)
	if authType != "bearer" {
		t.Errorf("expected auth type bearer, got %s", authType)
	}

	// Verify tools
	if len(manifest.Tools) != 5 {
		t.Fatalf("expected 5 tools, got %d", len(manifest.Tools))
	}

	expectedTools := []string{
		"list_agents",
		"get_agent",
		"get_discovery",
		"list_mcp_servers",
		"get_model_config",
	}
	for i, expected := range expectedTools {
		if manifest.Tools[i].Name != expected {
			t.Errorf("tool[%d]: expected name %q, got %q", i, expected, manifest.Tools[i].Name)
		}
		if manifest.Tools[i].Description == "" {
			t.Errorf("tool[%d]: expected non-empty description", i)
		}
	}
}

func TestMCPManifestHandler_TrailingSlash(t *testing.T) {
	// externalURL with trailing slash should not produce double slash
	h := NewMCPManifestHandler("https://registry.example.com/")

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	w := httptest.NewRecorder()

	h.GetManifest(w, req)

	var manifest MCPManifest
	json.NewDecoder(w.Body).Decode(&manifest)

	transport := manifest.Transport["streamableHttp"].(map[string]interface{})
	url := transport["url"].(string)
	if url != "https://registry.example.com/mcp/v1" {
		t.Errorf("trailing slash should be handled: got %s", url)
	}
}

func TestMCPManifestHandler_EmptyURL(t *testing.T) {
	h := NewMCPManifestHandler("")

	req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
	w := httptest.NewRecorder()

	h.GetManifest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var manifest MCPManifest
	json.NewDecoder(w.Body).Decode(&manifest)

	transport := manifest.Transport["streamableHttp"].(map[string]interface{})
	url := transport["url"].(string)
	if url != "/mcp/v1" {
		t.Errorf("empty URL should produce relative path: got %s", url)
	}
}
