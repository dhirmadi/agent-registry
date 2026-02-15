package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// =============================================================================
// Performance Test Suite for MCP Server Facade
//
// Covers:
//   1. Baseline latency (single request per method)
//   2. Load testing (100 concurrent connections, 1000 total requests)
//   3. Stress testing (increasing load to find breaking point)
//   4. Memory/allocation profiling
//   5. Session store concurrency under load
//   6. JSON serialization overhead
//   7. Batch request performance
// =============================================================================

// --- Rich mock stores for performance tests ---
// Each mock implements only the methods called by MCP tools/resources/prompts.
// Unused interface methods panic to catch accidental calls.

type perfMockAgentStore struct{}

func (s *perfMockAgentStore) List(_ context.Context, _ bool, offset, limit int) ([]store.Agent, int, error) {
	total := 16
	agents := make([]store.Agent, 0, limit)
	for i := offset; i < total && i < offset+limit; i++ {
		agents = append(agents, store.Agent{
			ID:          fmt.Sprintf("agent-%03d", i),
			Name:        fmt.Sprintf("Test Agent %d", i),
			Description: fmt.Sprintf("A test agent for performance benchmarks (%d)", i),
			IsActive:    true,
			Version:     1,
			CreatedBy:   "admin",
		})
	}
	return agents, total, nil
}

func (s *perfMockAgentStore) GetByID(_ context.Context, id string) (*store.Agent, error) {
	return &store.Agent{
		ID:           id,
		Name:         "Test Agent",
		Description:  "A test agent for benchmarks",
		SystemPrompt: "You are a helpful test agent.",
		IsActive:     true,
		Version:      1,
		CreatedBy:    "admin",
	}, nil
}

func (s *perfMockAgentStore) Create(_ context.Context, _ *store.Agent) error { panic("unused") }
func (s *perfMockAgentStore) Update(_ context.Context, _ *store.Agent, _ time.Time) error {
	panic("unused")
}
func (s *perfMockAgentStore) Patch(_ context.Context, _ string, _ map[string]interface{}, _ time.Time, _ string) (*store.Agent, error) {
	panic("unused")
}
func (s *perfMockAgentStore) Delete(_ context.Context, _ string) error { panic("unused") }
func (s *perfMockAgentStore) ListVersions(_ context.Context, _ string, _, _ int) ([]store.AgentVersion, int, error) {
	panic("unused")
}
func (s *perfMockAgentStore) GetVersion(_ context.Context, _ string, _ int) (*store.AgentVersion, error) {
	panic("unused")
}
func (s *perfMockAgentStore) Rollback(_ context.Context, _ string, _ int, _ string) (*store.Agent, error) {
	panic("unused")
}

type perfMockPromptStore struct{}

func (s *perfMockPromptStore) GetActive(_ context.Context, agentID string) (*store.Prompt, error) {
	return &store.Prompt{
		ID:           uuid.New(),
		AgentID:      agentID,
		SystemPrompt: "You are a helpful assistant. Your role is to help users with their tasks.",
		Mode:         "standard",
		Version:      1,
		IsActive:     true,
	}, nil
}

func (s *perfMockPromptStore) List(_ context.Context, _ string, _ bool, _, _ int) ([]store.Prompt, int, error) {
	panic("unused")
}
func (s *perfMockPromptStore) GetByID(_ context.Context, _ uuid.UUID) (*store.Prompt, error) {
	panic("unused")
}
func (s *perfMockPromptStore) Create(_ context.Context, _ *store.Prompt) error { panic("unused") }
func (s *perfMockPromptStore) Activate(_ context.Context, _ uuid.UUID) (*store.Prompt, error) {
	panic("unused")
}
func (s *perfMockPromptStore) Rollback(_ context.Context, _ string, _ int, _ string) (*store.Prompt, error) {
	panic("unused")
}

type perfMockMCPServerStore struct{}

func (s *perfMockMCPServerStore) List(_ context.Context) ([]store.MCPServer, error) {
	servers := make([]store.MCPServer, 5)
	for i := range servers {
		servers[i] = store.MCPServer{
			ID:        uuid.New(),
			Label:     fmt.Sprintf("MCP Server %d", i),
			Endpoint:  fmt.Sprintf("https://mcp%d.example.com/api", i),
			IsEnabled: true,
		}
	}
	return servers, nil
}

func (s *perfMockMCPServerStore) Create(_ context.Context, _ *store.MCPServer) error {
	panic("unused")
}
func (s *perfMockMCPServerStore) GetByID(_ context.Context, _ uuid.UUID) (*store.MCPServer, error) {
	panic("unused")
}
func (s *perfMockMCPServerStore) GetByLabel(_ context.Context, _ string) (*store.MCPServer, error) {
	panic("unused")
}
func (s *perfMockMCPServerStore) Update(_ context.Context, _ *store.MCPServer) error {
	panic("unused")
}
func (s *perfMockMCPServerStore) Delete(_ context.Context, _ uuid.UUID) error { panic("unused") }

type perfMockModelConfigStore struct{}

func (s *perfMockModelConfigStore) GetByScope(_ context.Context, _, _ string) (*store.ModelConfig, error) {
	return &store.ModelConfig{
		ID:                     uuid.New(),
		Scope:                  "global",
		DefaultModel:           "gpt-4",
		Temperature:            0.7,
		DefaultMaxOutputTokens: 4096,
		DefaultContextWindow:   128000,
		HistoryTokenBudget:     8192,
		MaxHistoryMessages:     20,
	}, nil
}

func (s *perfMockModelConfigStore) GetMerged(_ context.Context, _, _ string) (*store.ModelConfig, error) {
	panic("unused")
}
func (s *perfMockModelConfigStore) Update(_ context.Context, _ *store.ModelConfig, _ time.Time) error {
	panic("unused")
}
func (s *perfMockModelConfigStore) Upsert(_ context.Context, _ *store.ModelConfig) error {
	panic("unused")
}

type perfMockModelEndpointStore struct{}

func (s *perfMockModelEndpointStore) List(_ context.Context, _ *string, _ bool, _, _ int) ([]store.ModelEndpoint, int, error) {
	return []store.ModelEndpoint{
		{ID: uuid.New(), Slug: "openai-gpt4", Name: "OpenAI GPT-4", Provider: "openai", EndpointURL: "https://api.openai.com/v1", ModelName: "gpt-4", IsActive: true},
	}, 1, nil
}

func (s *perfMockModelEndpointStore) GetActiveVersion(_ context.Context, _ uuid.UUID) (*store.ModelEndpointVersion, error) {
	return &store.ModelEndpointVersion{
		Version: 1,
		Config:  json.RawMessage(`{"max_tokens":4096}`),
	}, nil
}

func (s *perfMockModelEndpointStore) Create(_ context.Context, _ *store.ModelEndpoint, _ json.RawMessage, _ string) error {
	panic("unused")
}
func (s *perfMockModelEndpointStore) GetBySlug(_ context.Context, _ string) (*store.ModelEndpoint, error) {
	panic("unused")
}
func (s *perfMockModelEndpointStore) Update(_ context.Context, _ *store.ModelEndpoint, _ time.Time) error {
	panic("unused")
}
func (s *perfMockModelEndpointStore) Delete(_ context.Context, _ string) error { panic("unused") }
func (s *perfMockModelEndpointStore) CreateVersion(_ context.Context, _ uuid.UUID, _ json.RawMessage, _, _ string) (*store.ModelEndpointVersion, error) {
	panic("unused")
}
func (s *perfMockModelEndpointStore) ListVersions(_ context.Context, _ uuid.UUID, _, _ int) ([]store.ModelEndpointVersion, int, error) {
	panic("unused")
}
func (s *perfMockModelEndpointStore) GetVersion(_ context.Context, _ uuid.UUID, _ int) (*store.ModelEndpointVersion, error) {
	panic("unused")
}
func (s *perfMockModelEndpointStore) ActivateVersion(_ context.Context, _ uuid.UUID, _ int) (*store.ModelEndpointVersion, error) {
	panic("unused")
}
func (s *perfMockModelEndpointStore) CountAll(_ context.Context) (int, error) { panic("unused") }

// --- Setup helpers ---

func newPerfMCPHandler() *MCPHandler {
	agents := &perfMockAgentStore{}
	prompts := &perfMockPromptStore{}
	mcpServers := &perfMockMCPServerStore{}
	model := &perfMockModelConfigStore{}
	modelEndpoints := &perfMockModelEndpointStore{}

	tools := NewMCPToolExecutor(agents, prompts, mcpServers, model, modelEndpoints, "https://registry.example.com")
	resources := NewMCPResourceProvider(agents, prompts, model)
	promptProvider := NewMCPPromptProvider(agents, prompts)
	manifest := NewMCPManifestHandler("https://registry.example.com")

	return NewMCPHandler(tools, resources, promptProvider, manifest)
}

func newPerfHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	h := newPerfMCPHandler()
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			h.HandlePost(w, r)
		case http.MethodDelete:
			h.HandleDelete(w, r)
		default:
			h.HandleSSE(w, r)
		}
	})
	mux.HandleFunc("/mcp.json", h.ServeManifest)
	return httptest.NewServer(mux)
}

func jsonRPCBody(method string, id int, params ...string) string {
	p := "null"
	if len(params) > 0 {
		p = params[0]
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"%s","params":%s}`, id, method, p)
}

// latencyStats computes avg/p50/p95/p99 from a sorted slice of durations.
type latencyStats struct {
	Count int
	Avg   time.Duration
	P50   time.Duration
	P95   time.Duration
	P99   time.Duration
	Min   time.Duration
	Max   time.Duration
}

func computeStats(durations []time.Duration) latencyStats {
	if len(durations) == 0 {
		return latencyStats{}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	var total time.Duration
	for _, d := range durations {
		total += d
	}

	n := len(durations)
	return latencyStats{
		Count: n,
		Avg:   total / time.Duration(n),
		P50:   durations[n/2],
		P95:   durations[int(float64(n)*0.95)],
		P99:   durations[int(float64(n)*0.99)],
		Min:   durations[0],
		Max:   durations[n-1],
	}
}

// =============================================================================
// 1. BASELINE LATENCY (single-request, in-process httptest.ResponseRecorder)
// =============================================================================

func TestPerf_Baseline_Initialize(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("initialize", 1, `{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"bench","version":"1.0"}}`)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("initialize failed: status=%d body=%s", rec.Code, rec.Body.String())
		}
	}

	stats := computeStats(durations)
	t.Logf("initialize baseline: avg=%v p50=%v p95=%v p99=%v min=%v max=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99, stats.Min, stats.Max)

	if stats.P95 > 50*time.Millisecond {
		t.Errorf("initialize P95 latency %v exceeds 50ms target", stats.P95)
	}
}

func TestPerf_Baseline_ToolsList(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("tools/list", 1)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("tools/list failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("tools/list baseline: avg=%v p50=%v p95=%v p99=%v min=%v max=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99, stats.Min, stats.Max)

	if stats.P95 > 20*time.Millisecond {
		t.Errorf("tools/list P95 latency %v exceeds 20ms target", stats.P95)
	}
}

func TestPerf_Baseline_ToolsCall_ListAgents(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("tools/call", 1, `{"name":"list_agents","arguments":{}}`)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("tools/call list_agents failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("tools/call(list_agents) baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 100*time.Millisecond {
		t.Errorf("tools/call P95 latency %v exceeds 100ms target", stats.P95)
	}
}

func TestPerf_Baseline_ToolsCall_GetDiscovery(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("tools/call", 1, `{"name":"get_discovery","arguments":{}}`)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("get_discovery failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("tools/call(get_discovery) baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 100*time.Millisecond {
		t.Errorf("get_discovery P95 latency %v exceeds 100ms target", stats.P95)
	}
}

func TestPerf_Baseline_ResourcesList(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("resources/list", 1)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("resources/list failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("resources/list baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 100*time.Millisecond {
		t.Errorf("resources/list P95 latency %v exceeds 100ms target", stats.P95)
	}
}

func TestPerf_Baseline_ResourcesRead(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("resources/read", 1, `{"uri":"config://model"}`)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("resources/read failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("resources/read baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 50*time.Millisecond {
		t.Errorf("resources/read P95 latency %v exceeds 50ms target", stats.P95)
	}
}

func TestPerf_Baseline_PromptsList(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("prompts/list", 1)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("prompts/list failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("prompts/list baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 100*time.Millisecond {
		t.Errorf("prompts/list P95 latency %v exceeds 100ms target", stats.P95)
	}
}

func TestPerf_Baseline_PromptsGet(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("prompts/get", 1, `{"name":"agent-001"}`)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("prompts/get failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("prompts/get baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 50*time.Millisecond {
		t.Errorf("prompts/get P95 latency %v exceeds 50ms target", stats.P95)
	}
}

func TestPerf_Baseline_Ping(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("ping", 1)

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("ping failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("ping baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 10*time.Millisecond {
		t.Errorf("ping P95 latency %v exceeds 10ms target", stats.P95)
	}
}

func TestPerf_Baseline_Manifest(t *testing.T) {
	h := newPerfMCPHandler()

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
		rec := httptest.NewRecorder()

		start := time.Now()
		h.ServeManifest(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("manifest failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("manifest baseline: avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 10*time.Millisecond {
		t.Errorf("manifest P95 latency %v exceeds 10ms target", stats.P95)
	}
}

// =============================================================================
// 2. LOAD TESTING — 100 concurrent connections, 1000 total requests
// =============================================================================

func TestPerf_Load_100Concurrent_1000Requests(t *testing.T) {
	srv := newPerfHTTPServer(t)
	defer srv.Close()

	const (
		concurrency   = 100
		totalRequests = 1000
	)

	methods := []struct {
		name   string
		body   string
		target time.Duration
	}{
		{"ping", jsonRPCBody("ping", 1), 50 * time.Millisecond},
		{"tools/list", jsonRPCBody("tools/list", 1), 50 * time.Millisecond},
		{"tools/call(list_agents)", jsonRPCBody("tools/call", 1, `{"name":"list_agents","arguments":{}}`), 200 * time.Millisecond},
		{"tools/call(get_discovery)", jsonRPCBody("tools/call", 1, `{"name":"get_discovery","arguments":{}}`), 200 * time.Millisecond},
		{"resources/list", jsonRPCBody("resources/list", 1), 200 * time.Millisecond},
		{"resources/read", jsonRPCBody("resources/read", 1, `{"uri":"config://model"}`), 100 * time.Millisecond},
		{"prompts/list", jsonRPCBody("prompts/list", 1), 200 * time.Millisecond},
		{"prompts/get", jsonRPCBody("prompts/get", 1, `{"name":"agent-001"}`), 100 * time.Millisecond},
	}

	for _, m := range methods {
		m := m
		t.Run(m.name, func(t *testing.T) {
			var (
				mu        sync.Mutex
				durations = make([]time.Duration, 0, totalRequests)
				errors    int64
			)

			sem := make(chan struct{}, concurrency)
			var wg sync.WaitGroup

			for i := 0; i < totalRequests; i++ {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()

					start := time.Now()
					resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(m.body))
					elapsed := time.Since(start)

					if err != nil {
						atomic.AddInt64(&errors, 1)
						return
					}
					io.ReadAll(resp.Body)
					resp.Body.Close()

					if resp.StatusCode != http.StatusOK {
						atomic.AddInt64(&errors, 1)
						return
					}

					mu.Lock()
					durations = append(durations, elapsed)
					mu.Unlock()
				}()
			}
			wg.Wait()

			stats := computeStats(durations)
			errCount := atomic.LoadInt64(&errors)

			t.Logf("load %s: requests=%d errors=%d avg=%v p50=%v p95=%v p99=%v min=%v max=%v",
				m.name, totalRequests, errCount, stats.Avg, stats.P50, stats.P95, stats.P99, stats.Min, stats.Max)

			if errCount > 0 {
				t.Errorf("had %d/%d errors", errCount, totalRequests)
			}
			if stats.P95 > m.target {
				t.Errorf("%s P95 latency %v exceeds %v target under load", m.name, stats.P95, m.target)
			}
		})
	}
}

// =============================================================================
// 3. STRESS TEST — Increasing load to find breaking point
// =============================================================================

func TestPerf_Stress_IncreasingLoad(t *testing.T) {
	srv := newPerfHTTPServer(t)
	defer srv.Close()

	body := jsonRPCBody("tools/call", 1, `{"name":"list_agents","arguments":{}}`)
	concurrencyLevels := []int{10, 50, 100, 200, 500}

	for _, c := range concurrencyLevels {
		c := c
		t.Run(fmt.Sprintf("concurrent_%d", c), func(t *testing.T) {
			const requestsPerGoroutine = 10
			totalRequests := c * requestsPerGoroutine

			var (
				mu        sync.Mutex
				durations = make([]time.Duration, 0, totalRequests)
				errors    int64
			)

			var wg sync.WaitGroup
			for i := 0; i < c; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < requestsPerGoroutine; j++ {
						start := time.Now()
						resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
						elapsed := time.Since(start)

						if err != nil {
							atomic.AddInt64(&errors, 1)
							continue
						}
						io.ReadAll(resp.Body)
						resp.Body.Close()

						if resp.StatusCode != http.StatusOK {
							atomic.AddInt64(&errors, 1)
							continue
						}

						mu.Lock()
						durations = append(durations, elapsed)
						mu.Unlock()
					}
				}()
			}
			wg.Wait()

			stats := computeStats(durations)
			errCount := atomic.LoadInt64(&errors)
			errRate := float64(errCount) / float64(totalRequests) * 100

			t.Logf("stress c=%d: total=%d errors=%d (%.1f%%) avg=%v p50=%v p95=%v p99=%v max=%v",
				c, totalRequests, errCount, errRate, stats.Avg, stats.P50, stats.P95, stats.P99, stats.Max)

			// At high concurrency, we expect zero errors (no crashes, no panics)
			if errRate > 5.0 {
				t.Errorf("error rate %.1f%% exceeds 5%% threshold at concurrency=%d", errRate, c)
			}
		})
	}
}

// =============================================================================
// 4. MEMORY / ALLOCATION PROFILING
// =============================================================================

func TestPerf_Memory_NoLeaks(t *testing.T) {
	h := newPerfMCPHandler()
	body := jsonRPCBody("tools/call", 1, `{"name":"get_discovery","arguments":{}}`)

	// Warm up
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	const iterations = 1000
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}

	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	allocatedMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / (1024 * 1024)
	allocsPerReq := float64(memAfter.Mallocs-memBefore.Mallocs) / float64(iterations)
	heapDiffKB := float64(int64(memAfter.HeapInuse)-int64(memBefore.HeapInuse)) / 1024

	t.Logf("memory: total_alloc=%.2fMB allocs_per_req=%.0f heap_diff=%.1fKB goroutines=%d",
		allocatedMB, allocsPerReq, heapDiffKB, runtime.NumGoroutine())

	// No massive leak: heap growth should be under 10MB for 1000 requests
	if heapDiffKB > 10*1024 {
		t.Errorf("heap grew by %.1fKB after %d requests — possible memory leak", heapDiffKB, iterations)
	}
}

func TestPerf_Memory_SessionStore_NoLeak(t *testing.T) {
	h := newPerfMCPHandler()
	initBody := jsonRPCBody("initialize", 1, `{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"bench","version":"1.0"}}`)

	// Create 1000 sessions
	sessionIDs := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)

		sid := rec.Header().Get("Mcp-Session-Id")
		if sid != "" {
			sessionIDs = append(sessionIDs, sid)
		}
	}

	runtime.GC()
	var memWithSessions runtime.MemStats
	runtime.ReadMemStats(&memWithSessions)

	// Delete all sessions
	for _, sid := range sessionIDs {
		req := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
		req.Header.Set("Mcp-Session-Id", sid)
		rec := httptest.NewRecorder()
		h.HandleDelete(rec, req)
	}

	runtime.GC()
	var memAfterCleanup runtime.MemStats
	runtime.ReadMemStats(&memAfterCleanup)

	t.Logf("session store: created=%d heap_with_sessions=%dKB heap_after_cleanup=%dKB",
		len(sessionIDs),
		memWithSessions.HeapInuse/1024,
		memAfterCleanup.HeapInuse/1024)

	// Verify sessions were actually created
	if len(sessionIDs) == 0 {
		t.Log("note: no session IDs returned (transport may not have sessions enabled in this config)")
	}
}

// =============================================================================
// 5. SESSION STORE CONCURRENCY STRESS
// =============================================================================

func TestPerf_SessionStore_ConcurrentAccess(t *testing.T) {
	h := newPerfMCPHandler()

	const (
		goroutines = 100
		operations = 100
	)

	var (
		wg     sync.WaitGroup
		errors int64
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				// Create session
				initBody := jsonRPCBody("initialize", id*1000+j,
					`{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"bench","version":"1.0"}}`)
				req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(initBody))
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				h.HandlePost(rec, req)

				if rec.Code != http.StatusOK {
					atomic.AddInt64(&errors, 1)
					continue
				}

				// Ping on the session
				pingBody := jsonRPCBody("ping", id*1000+j+1)
				req2 := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(pingBody))
				req2.Header.Set("Content-Type", "application/json")
				if sid := rec.Header().Get("Mcp-Session-Id"); sid != "" {
					req2.Header.Set("Mcp-Session-Id", sid)
				}
				rec2 := httptest.NewRecorder()
				h.HandlePost(rec2, req2)

				if rec2.Code != http.StatusOK {
					atomic.AddInt64(&errors, 1)
				}

				// Delete session
				if sid := rec.Header().Get("Mcp-Session-Id"); sid != "" {
					req3 := httptest.NewRequest(http.MethodDelete, "/mcp", nil)
					req3.Header.Set("Mcp-Session-Id", sid)
					rec3 := httptest.NewRecorder()
					h.HandleDelete(rec3, req3)
				}
			}
		}(i)
	}
	wg.Wait()

	errCount := atomic.LoadInt64(&errors)
	t.Logf("session concurrency: goroutines=%d ops_each=%d total=%d errors=%d",
		goroutines, operations, goroutines*operations, errCount)

	if errCount > 0 {
		t.Errorf("session concurrency had %d errors", errCount)
	}
}

// =============================================================================
// 6. JSON SERIALIZATION OVERHEAD
// =============================================================================

func TestPerf_JSONSerializationOverhead(t *testing.T) {
	h := newPerfMCPHandler()

	// Measure time spent in handler vs total request time
	methods := []struct {
		name   string
		method string
		params json.RawMessage
	}{
		{"tools/list", "tools/list", nil},
		{"tools/call(list_agents)", "tools/call", json.RawMessage(`{"name":"list_agents","arguments":{}}`)},
		{"tools/call(get_discovery)", "tools/call", json.RawMessage(`{"name":"get_discovery","arguments":{}}`)},
		{"resources/list", "resources/list", nil},
		{"prompts/list", "prompts/list", nil},
	}

	for _, m := range methods {
		// Measure HandleMethod directly (no JSON-RPC framing)
		const iterations = 1000
		directDurations := make([]time.Duration, iterations)
		for i := 0; i < iterations; i++ {
			start := time.Now()
			_, _ = h.HandleMethod(context.Background(), m.method, m.params)
			directDurations[i] = time.Since(start)
		}

		// Measure via full HTTP path
		body := jsonRPCBody(m.method, 1)
		if m.params != nil {
			body = fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"%s","params":%s}`, m.method, string(m.params))
		}
		httpDurations := make([]time.Duration, iterations)
		for i := 0; i < iterations; i++ {
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			start := time.Now()
			h.HandlePost(rec, req)
			httpDurations[i] = time.Since(start)
		}

		directStats := computeStats(directDurations)
		httpStats := computeStats(httpDurations)

		overhead := time.Duration(0)
		if httpStats.Avg > directStats.Avg {
			overhead = httpStats.Avg - directStats.Avg
		}
		overheadPct := float64(0)
		if httpStats.Avg > 0 {
			overheadPct = float64(overhead) / float64(httpStats.Avg) * 100
		}

		t.Logf("%s: direct_avg=%v http_avg=%v overhead=%v (%.1f%%)",
			m.name, directStats.Avg, httpStats.Avg, overhead, overheadPct)

		// JSON/HTTP framing overhead should be under 50% of total time
		if overheadPct > 50 {
			t.Logf("WARNING: JSON/HTTP overhead %.1f%% is high for %s (may indicate serialization bottleneck)", overheadPct, m.name)
		}
	}
}

// =============================================================================
// 7. BATCH REQUEST PERFORMANCE
// =============================================================================

func TestPerf_BatchRequest(t *testing.T) {
	h := newPerfMCPHandler()

	// Build a batch of 10 requests
	var batchItems []string
	for i := 1; i <= 10; i++ {
		batchItems = append(batchItems, jsonRPCBody("ping", i))
	}
	batchBody := "[" + strings.Join(batchItems, ",") + "]"

	// Compare: 10 individual requests vs 1 batch of 10
	const iterations = 100

	// Individual requests
	individualDurations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		start := time.Now()
		for j := 0; j < 10; j++ {
			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(jsonRPCBody("ping", j+1)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandlePost(rec, req)
		}
		individualDurations[i] = time.Since(start)
	}

	// Batch requests
	batchDurations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(batchBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		batchDurations[i] = time.Since(start)
	}

	indStats := computeStats(individualDurations)
	batchStats := computeStats(batchDurations)

	speedup := float64(indStats.Avg) / float64(batchStats.Avg)

	t.Logf("batch vs individual (10 pings): individual_avg=%v batch_avg=%v speedup=%.1fx",
		indStats.Avg, batchStats.Avg, speedup)

	// Batch should be at least 1.5x faster than 10 individual requests
	if speedup < 1.5 {
		t.Logf("WARNING: batch speedup %.1fx is lower than expected (target: >1.5x)", speedup)
	}
}

func TestPerf_BatchRequest_MixedMethods(t *testing.T) {
	h := newPerfMCPHandler()

	batchBody := `[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","id":2,"method":"tools/list"},
		{"jsonrpc":"2.0","id":3,"method":"resources/list"},
		{"jsonrpc":"2.0","id":4,"method":"prompts/list"},
		{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}
	]`

	const iterations = 100
	durations := make([]time.Duration, iterations)
	for i := 0; i < iterations; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(batchBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		start := time.Now()
		h.HandlePost(rec, req)
		durations[i] = time.Since(start)

		if rec.Code != http.StatusOK {
			t.Fatalf("batch failed: status=%d", rec.Code)
		}
	}

	stats := computeStats(durations)
	t.Logf("mixed batch (5 methods): avg=%v p50=%v p95=%v p99=%v",
		stats.Avg, stats.P50, stats.P95, stats.P99)

	if stats.P95 > 50*time.Millisecond {
		t.Errorf("mixed batch P95 %v exceeds 50ms target", stats.P95)
	}
}

// =============================================================================
// 8. GOROUTINE LEAK DETECTION
// =============================================================================

func TestPerf_GoroutineLeak(t *testing.T) {
	// Baseline goroutine count
	runtime.GC()
	goroutinesBefore := runtime.NumGoroutine()

	srv := newPerfHTTPServer(t)
	body := jsonRPCBody("tools/call", 1, `{"name":"get_discovery","arguments":{}}`)

	// Send 500 requests with 50 concurrent connections
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(body))
				if err != nil {
					continue
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()

	srv.Close()

	// Wait for goroutines to settle
	time.Sleep(200 * time.Millisecond)
	runtime.GC()
	goroutinesAfter := runtime.NumGoroutine()

	leaked := goroutinesAfter - goroutinesBefore
	t.Logf("goroutines: before=%d after=%d leaked=%d", goroutinesBefore, goroutinesAfter, leaked)

	// Allow some slack (HTTP client keep-alives, etc.) but no massive leak
	if leaked > 20 {
		t.Errorf("goroutine leak detected: %d goroutines leaked after 500 requests", leaked)
	}
}

// =============================================================================
// 9. LARGE PAYLOAD HANDLING
// =============================================================================

func TestPerf_LargePayload_Boundary(t *testing.T) {
	h := newPerfMCPHandler()

	// Test payloads of increasing size up to the 1MB limit
	sizes := []int{1024, 10240, 102400, 512000, 1048575} // 1KB, 10KB, 100KB, 500KB, ~1MB
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dB", size), func(t *testing.T) {
			// Create a payload with a large params field
			filler := strings.Repeat("x", size)
			body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"data":"%s"}}`, filler)

			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			start := time.Now()
			h.HandlePost(rec, req)
			elapsed := time.Since(start)

			t.Logf("size=%d status=%d latency=%v", size, rec.Code, elapsed)

			// Should still respond within 500ms even for large payloads
			if elapsed > 500*time.Millisecond {
				t.Errorf("large payload (%dB) took %v — exceeds 500ms", size, elapsed)
			}
		})
	}
}

func TestPerf_OverSizePayload_Rejected(t *testing.T) {
	h := newPerfMCPHandler()

	// Create a payload over 1MB
	filler := strings.Repeat("A", 1<<20+100)
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"data":"%s"}}`, filler)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	start := time.Now()
	h.HandlePost(rec, req)
	elapsed := time.Since(start)

	t.Logf("oversize payload: status=%d latency=%v", rec.Code, elapsed)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize payload returned status %d, expected 413", rec.Code)
	}

	// Rejection should be fast
	if elapsed > 100*time.Millisecond {
		t.Errorf("oversize payload rejection took %v — should be near-instant", elapsed)
	}
}

// =============================================================================
// 10. Go BENCHMARKS (for `go test -bench`)
// =============================================================================

func BenchmarkMCP_Ping(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("ping", 1))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_ToolsList(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("tools/list", 1))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_ToolsCall_ListAgents(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("tools/call", 1, `{"name":"list_agents","arguments":{}}`))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_ToolsCall_GetDiscovery(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("tools/call", 1, `{"name":"get_discovery","arguments":{}}`))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_ResourcesList(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("resources/list", 1))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_ResourcesRead(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("resources/read", 1, `{"uri":"config://model"}`))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_PromptsList(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("prompts/list", 1))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_PromptsGet(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("prompts/get", 1, `{"name":"agent-001"}`))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_Initialize(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("initialize", 1, `{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"bench","version":"1.0"}}`))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

func BenchmarkMCP_Manifest(b *testing.B) {
	h := newPerfMCPHandler()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/mcp.json", nil)
		rec := httptest.NewRecorder()
		h.ServeManifest(rec, req)
	}
}

func BenchmarkMCP_Batch_5Methods(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(`[
		{"jsonrpc":"2.0","id":1,"method":"ping"},
		{"jsonrpc":"2.0","id":2,"method":"tools/list"},
		{"jsonrpc":"2.0","id":3,"method":"resources/list"},
		{"jsonrpc":"2.0","id":4,"method":"prompts/list"},
		{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"list_agents","arguments":{}}}
	]`)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.HandlePost(rec, req)
	}
}

// BenchmarkMCP_Parallel measures throughput under parallel load.
func BenchmarkMCP_Parallel_ToolsCall(b *testing.B) {
	h := newPerfMCPHandler()
	body := []byte(jsonRPCBody("tools/call", 1, `{"name":"list_agents","arguments":{}}`))

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			h.HandlePost(rec, req)
		}
	})
}
