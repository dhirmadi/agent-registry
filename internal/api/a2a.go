package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	apierrors "github.com/agent-smit/agentic-registry/internal/errors"
	"github.com/agent-smit/agentic-registry/internal/store"
)

// A2A protocol version supported by this implementation.
const a2aProtocolVersion = "0.3.0"

// A2AAgentCard represents an A2A protocol Agent Card (v0.3.0).
type A2AAgentCard struct {
	Name               string                       `json:"name"`
	Description        string                       `json:"description"`
	URL                string                       `json:"url"`
	Version            string                       `json:"version"`
	ProtocolVersion    string                       `json:"protocolVersion"`
	Provider           A2AProvider                  `json:"provider"`
	Capabilities       A2ACapabilities              `json:"capabilities"`
	DefaultInputModes  []string                     `json:"defaultInputModes"`
	DefaultOutputModes []string                     `json:"defaultOutputModes"`
	Skills             []A2ASkill                   `json:"skills"`
	SecuritySchemes    map[string]A2ASecurityScheme `json:"securitySchemes"`
	Security           []map[string][]string        `json:"security"`
}

// A2AProvider identifies the organization hosting the agent.
type A2AProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url"`
}

// A2ACapabilities describes what the agent supports.
type A2ACapabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
}

// A2ASkill represents a single capability of an agent.
type A2ASkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Examples    []string `json:"examples"`
}

// A2ASecurityScheme describes an authentication method.
type A2ASecurityScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme"`
}

// A2AHandler provides HTTP handlers for A2A protocol endpoints.
type A2AHandler struct {
	agents      AgentStoreForAPI
	externalURL string
}

// NewA2AHandler creates a new A2AHandler.
func NewA2AHandler(agents AgentStoreForAPI, externalURL string) *A2AHandler {
	return &A2AHandler{
		agents:      agents,
		externalURL: externalURL,
	}
}

// agentToA2ACard maps a store.Agent to an A2AAgentCard.
func agentToA2ACard(agent *store.Agent, externalURL string) A2AAgentCard {
	card := A2AAgentCard{
		Name:            agent.Name,
		Description:     agent.Description,
		URL:             externalURL + "/api/v1/agents/" + agent.ID,
		Version:         strconv.Itoa(agent.Version),
		ProtocolVersion: a2aProtocolVersion,
		Provider: A2AProvider{
			Organization: "Agentic Registry",
			URL:          externalURL,
		},
		Capabilities:       A2ACapabilities{},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		SecuritySchemes: map[string]A2ASecurityScheme{
			"bearerAuth": {Type: "http", Scheme: "bearer"},
		},
		Security: []map[string][]string{
			{"bearerAuth": {}},
		},
	}

	// Parse tools to create skills
	var tools []agentTool
	if len(agent.Tools) > 0 {
		json.Unmarshal(agent.Tools, &tools)
	}

	// Parse example prompts
	var examples []string
	if len(agent.ExamplePrompts) > 0 {
		json.Unmarshal(agent.ExamplePrompts, &examples)
	}

	if len(tools) == 0 {
		// No tools: create a synthetic skill from the agent itself
		card.Skills = []A2ASkill{
			{
				ID:          agent.ID,
				Name:        agent.Name,
				Description: agent.Description,
				Tags:        []string{},
				Examples:    examples,
			},
		}
	} else {
		card.Skills = make([]A2ASkill, 0, len(tools))
		for i, tool := range tools {
			tags := []string{tool.Source}
			if tool.ServerLabel != "" {
				tags = append(tags, tool.ServerLabel)
			}
			skill := A2ASkill{
				ID:          tool.Name,
				Name:        tool.Name,
				Description: tool.Description,
				Tags:        tags,
				Examples:    []string{},
			}
			// Attach example prompts to the first skill only
			if i == 0 {
				skill.Examples = examples
			}
			card.Skills = append(card.Skills, skill)
		}
	}

	return card
}

// GetAgentCard handles GET /api/v1/agents/{agentId}/agent-card.
func (h *A2AHandler) GetAgentCard(w http.ResponseWriter, r *http.Request) {
	agentID := chi.URLParam(r, "agentId")

	agent, err := h.agents.GetByID(r.Context(), agentID)
	if err != nil {
		if isNotFoundError(err) {
			RespondError(w, r, apierrors.NotFound("agent", agentID))
		} else {
			RespondError(w, r, apierrors.Internal("failed to retrieve agent"))
		}
		return
	}

	card := agentToA2ACard(agent, h.externalURL)
	RespondJSON(w, r, http.StatusOK, card)
}

// GetWellKnownAgentCard handles GET /.well-known/agent.json.
// Returns raw JSON (no envelope) for A2A protocol compliance.
func (h *A2AHandler) GetWellKnownAgentCard(w http.ResponseWriter, r *http.Request) {
	agents, _, err := h.agents.List(r.Context(), true, 0, 1000)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Compute ETag from max(UpdatedAt)
	var maxUpdated time.Time
	for i := range agents {
		if agents[i].UpdatedAt.After(maxUpdated) {
			maxUpdated = agents[i].UpdatedAt
		}
	}
	etag := fmt.Sprintf(`"%s"`, maxUpdated.UTC().Format(time.RFC3339Nano))

	// Support conditional request
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Build skills from active agents
	skills := make([]A2ASkill, 0, len(agents))
	for _, agent := range agents {
		skills = append(skills, A2ASkill{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
			Tags:        []string{"agent"},
			Examples:    []string{},
		})
	}

	card := A2AAgentCard{
		Name:            "Agentic Registry",
		Description:     "A registry of AI agents and their configurations",
		URL:             h.externalURL,
		Version:         "1.0.0",
		ProtocolVersion: a2aProtocolVersion,
		Provider: A2AProvider{
			Organization: "Agentic Registry",
			URL:          h.externalURL,
		},
		Capabilities:       A2ACapabilities{},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Skills:             skills,
		SecuritySchemes: map[string]A2ASecurityScheme{
			"bearerAuth": {Type: "http", Scheme: "bearer"},
		},
		Security: []map[string][]string{
			{"bearerAuth": {}},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(card)
}

// GetA2AIndex handles GET /api/v1/agents/a2a-index.
func (h *A2AHandler) GetA2AIndex(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	agents, total, err := h.agents.List(r.Context(), true, offset, limit)
	if err != nil {
		RespondError(w, r, apierrors.Internal("failed to list agents"))
		return
	}

	// Map to A2A cards with optional keyword filter
	query := strings.ToLower(r.URL.Query().Get("q"))
	cards := make([]A2AAgentCard, 0, len(agents))
	for i := range agents {
		card := agentToA2ACard(&agents[i], h.externalURL)
		if query != "" {
			nameMatch := strings.Contains(strings.ToLower(card.Name), query)
			descMatch := strings.Contains(strings.ToLower(card.Description), query)
			if !nameMatch && !descMatch {
				continue
			}
		}
		cards = append(cards, card)
	}

	// When filtering with q, total reflects filtered count
	responseTotal := total
	if query != "" {
		responseTotal = len(cards)
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"agent_cards": cards,
		"total":       responseTotal,
	})
}
