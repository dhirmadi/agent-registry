package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestProxyClient_SuccessfulProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"result": map[string]interface{}{
				"content": []map[string]interface{}{
					{"type": "text", "text": "hello"},
				},
			},
		})
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{
		Timeout:             5 * time.Second,
		MaxIdleConnsPerHost: 2,
		AllowPrivateIPs:     true, // Allow localhost for testing
	})

	resp, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "greet",
		Arguments:      json.RawMessage(`{"name":"world"}`),
		AuthType:       "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if len(resp.Body) == 0 {
		t.Error("expected non-empty body")
	}
	if resp.Latency <= 0 {
		t.Error("expected positive latency")
	}
}

func TestProxyClient_BearerAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "bearer",
		AuthCredential: "my-secret-token",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

func TestProxyClient_BasicAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "basic",
		AuthCredential: "dXNlcjpwYXNz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Basic dXNlcjpwYXNz" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Basic dXNlcjpwYXNz")
	}
}

func TestProxyClient_NoAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty", gotAuth)
	}
}

func TestProxyClient_UnsupportedAuthType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "oauth2",
	})
	if err == nil {
		t.Fatal("expected error for unsupported auth type")
	}
	if !strings.Contains(err.Error(), "unsupported auth type") {
		t.Errorf("error = %q, want it to contain 'unsupported auth type'", err.Error())
	}
}

func TestProxyClient_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 50 * time.Millisecond, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "slow",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "none",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestProxyClient_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 10 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := pc.Forward(ctx, ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "none",
	})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestProxyClient_Non200Status(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"401 Unauthorized", http.StatusUnauthorized},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"upstream error"}}`))
			}))
			defer srv.Close()

			pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
			resp, err := pc.Forward(context.Background(), ProxyRequest{
				ServerEndpoint: srv.URL,
				ToolName:       "test",
				Arguments:      json.RawMessage(`{}`),
				AuthType:       "none",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.StatusCode != tc.statusCode {
				t.Errorf("status = %d, want %d", resp.StatusCode, tc.statusCode)
			}
		})
	}
}

func TestProxyClient_MalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("this is not json"))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	resp, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "none",
	})
	// The proxy client returns raw bytes; it does not parse JSON.
	// Caller (handler) decides how to interpret the response.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if string(resp.Body) != "this is not json" {
		t.Errorf("body = %q, want %q", string(resp.Body), "this is not json")
	}
}

func TestProxyClient_JSONRPCFormat(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method and content type
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}

		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "my_tool",
		Arguments:      json.RawMessage(`{"key":"value"}`),
		AuthType:       "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify JSON-RPC 2.0 structure
	if v, ok := receivedBody["jsonrpc"]; !ok || v != "2.0" {
		t.Errorf("jsonrpc = %v, want \"2.0\"", v)
	}
	if v, ok := receivedBody["method"]; !ok || v != "tools/call" {
		t.Errorf("method = %v, want \"tools/call\"", v)
	}
	if _, ok := receivedBody["id"]; !ok {
		t.Error("missing id field")
	}

	params, ok := receivedBody["params"].(map[string]interface{})
	if !ok {
		t.Fatal("params is not a map")
	}
	if params["name"] != "my_tool" {
		t.Errorf("params.name = %v, want \"my_tool\"", params["name"])
	}
	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		t.Fatal("params.arguments is not a map")
	}
	if args["key"] != "value" {
		t.Errorf("params.arguments.key = %v, want \"value\"", args["key"])
	}
}

func TestProxyClient_LatencyMeasurement(t *testing.T) {
	delay := 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	resp, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Latency should be at least the server delay
	if resp.Latency < delay {
		t.Errorf("latency = %v, want >= %v", resp.Latency, delay)
	}
	// Latency should be reasonable (not more than 2x the delay + overhead)
	if resp.Latency > 3*delay {
		t.Errorf("latency = %v, suspiciously high (expected ~%v)", resp.Latency, delay)
	}
}

func TestProxyClient_RequestResponseSizes(t *testing.T) {
	responseBody := `{"jsonrpc":"2.0","id":1,"result":{"data":"test-value"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseBody))
	}))
	defer srv.Close()

	pc := NewProxyClient(ProxyClientConfig{Timeout: 5 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	resp, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: srv.URL,
		ToolName:       "test",
		Arguments:      json.RawMessage(`{"key":"value"}`),
		AuthType:       "none",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RequestSize <= 0 {
		t.Error("expected positive request size")
	}
	if resp.ResponseSize != int64(len(responseBody)) {
		t.Errorf("response size = %d, want %d", resp.ResponseSize, len(responseBody))
	}
}

func TestProxyClient_ConnectionRefused(t *testing.T) {
	pc := NewProxyClient(ProxyClientConfig{Timeout: 2 * time.Second, MaxIdleConnsPerHost: 2, AllowPrivateIPs: true})
	_, err := pc.Forward(context.Background(), ProxyRequest{
		ServerEndpoint: "http://127.0.0.1:1", // port 1 is almost certainly not listening
		ToolName:       "test",
		Arguments:      json.RawMessage(`{}`),
		AuthType:       "none",
	})
	if err == nil {
		t.Fatal("expected connection error")
	}
}
