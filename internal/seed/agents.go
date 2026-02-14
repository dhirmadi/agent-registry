package seed

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/agent-smit/agentic-registry/internal/store"
)

// AgentStore defines the interface for creating agents.
type AgentStore interface {
	Create(ctx context.Context, agent *store.Agent) error
}

// AgentCounter defines the interface for counting agents.
type AgentCounter interface {
	CountAll(ctx context.Context) (int, error)
}

// SeedAgents populates the database with 16 default product agents on first boot.
// It checks if agents already exist and skips seeding if the table is not empty.
func SeedAgents(ctx context.Context, s interface {
	AgentStore
	AgentCounter
}) error {
	// Check if agents already exist
	count, err := s.CountAll(ctx)
	if err != nil {
		return fmt.Errorf("checking agent count: %w", err)
	}

	if count > 0 {
		// Skip seeding if agents already exist
		return nil
	}

	// Seed the 16 product agents
	agents := buildSeedAgents()
	for _, agent := range agents {
		if err := s.Create(ctx, agent); err != nil {
			return fmt.Errorf("creating agent %q: %w", agent.ID, err)
		}
	}

	return nil
}

// buildSeedAgents constructs the 16 product agents.
// First 6 have full tool definitions, remaining 10 are placeholders.
func buildSeedAgents() []*store.Agent {
	agents := make([]*store.Agent, 0, 16)

	// First 6 agents with full tool definitions
	agents = append(agents, &store.Agent{
		ID:          "router",
		Name:        "Router Agent",
		Description: "Routes incoming requests to the appropriate specialized agent based on intent classification.",
		SystemPrompt: `You are the Router Agent. Your role is to analyze incoming user requests and delegate them to the most appropriate specialized agent.

Key responsibilities:
- Classify user intent (PMO task, communication, meeting, RAID item, etc.)
- Route to the correct agent using delegate_to_agent
- Handle multi-intent requests by breaking them into sub-tasks
- Maintain conversation context across delegations`,
		Tools:          mustMarshalJSON(buildRouterTools()),
		TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
		ExamplePrompts: mustMarshalJSON([]string{
			"What's the status of Project Alpha?",
			"Schedule a meeting with the team for next Tuesday",
			"Add a new risk about resource constraints",
		}),
		IsActive:  true,
		CreatedBy: "system-seed",
	})

	agents = append(agents, &store.Agent{
		ID:          "pmo",
		Name:        "PMO Agent",
		Description: "Manages project portfolio, tracks deliverables, monitors timelines, and provides status reporting.",
		SystemPrompt: `You are the PMO Agent. You manage the project portfolio and provide oversight across all active projects.

Key responsibilities:
- Track project status, milestones, and deliverables
- Generate status reports and dashboards
- Monitor resource allocation and capacity
- Identify cross-project dependencies and conflicts
- Escalate issues requiring executive attention`,
		Tools:          mustMarshalJSON(buildPMOTools()),
		TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
		ExamplePrompts: mustMarshalJSON([]string{
			"Show me all projects due this quarter",
			"What's the overall portfolio health?",
			"List projects with red status",
		}),
		IsActive:  true,
		CreatedBy: "system-seed",
	})

	agents = append(agents, &store.Agent{
		ID:          "raid_manager",
		Name:        "RAID Manager",
		Description: "Tracks and manages Risks, Assumptions, Issues, and Dependencies (RAID) across projects.",
		SystemPrompt: `You are the RAID Manager. You maintain the RAID log and help teams proactively manage project challenges.

Key responsibilities:
- Log and categorize risks, assumptions, issues, and dependencies
- Assess impact and probability for risks
- Assign ownership and track mitigation actions
- Escalate high-priority items
- Generate RAID reports for stakeholders`,
		Tools:          mustMarshalJSON(buildRAIDTools()),
		TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
		ExamplePrompts: mustMarshalJSON([]string{
			"Log a new risk about vendor delays",
			"Show me all high-priority issues",
			"What dependencies does Project Beta have?",
		}),
		IsActive:  true,
		CreatedBy: "system-seed",
	})

	agents = append(agents, &store.Agent{
		ID:          "task_manager",
		Name:        "Task Manager",
		Description: "Manages tasks, action items, and work assignments across the team.",
		SystemPrompt: `You are the Task Manager. You help teams organize and track their work.

Key responsibilities:
- Create and assign tasks
- Track task status and progress
- Identify blockers and dependencies
- Remind team members of due dates
- Generate task reports and burndown charts`,
		Tools:          mustMarshalJSON(buildTaskTools()),
		TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
		ExamplePrompts: mustMarshalJSON([]string{
			"Create a task for updating the documentation",
			"Show me all tasks assigned to Sarah",
			"What tasks are overdue?",
		}),
		IsActive:  true,
		CreatedBy: "system-seed",
	})

	agents = append(agents, &store.Agent{
		ID:          "comms_manager",
		Name:        "Communications Manager",
		Description: "Handles internal and external communications, notifications, and stakeholder updates.",
		SystemPrompt: `You are the Communications Manager. You ensure effective communication across the organization.

Key responsibilities:
- Send messages via email, Slack, and other channels
- Draft stakeholder updates and announcements
- Manage distribution lists and notification preferences
- Track communication history
- Ensure timely delivery of critical updates`,
		Tools:          mustMarshalJSON(buildCommsTools()),
		TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
		ExamplePrompts: mustMarshalJSON([]string{
			"Send a project update to all stakeholders",
			"Notify the team about tomorrow's deployment",
			"Draft an announcement for the new feature release",
		}),
		IsActive:  true,
		CreatedBy: "system-seed",
	})

	agents = append(agents, &store.Agent{
		ID:          "meeting_manager",
		Name:        "Meeting Manager",
		Description: "Schedules meetings, manages calendars, prepares agendas, and tracks action items.",
		SystemPrompt: `You are the Meeting Manager. You coordinate meetings and ensure productive outcomes.

Key responsibilities:
- Schedule meetings and find optimal time slots
- Prepare and distribute agendas
- Track attendance and participation
- Capture decisions and action items
- Send meeting summaries and follow-ups`,
		Tools:          mustMarshalJSON(buildMeetingTools()),
		TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
		ExamplePrompts: mustMarshalJSON([]string{
			"Schedule a sprint planning meeting for next week",
			"Find a time when all executives are available",
			"Send the agenda for tomorrow's standup",
		}),
		IsActive:  true,
		CreatedBy: "system-seed",
	})

	// Remaining 10 placeholder agents
	placeholders := []struct {
		id          string
		name        string
		description string
		prompt      string
	}{
		{
			id:          "engagement_pm",
			name:        "Engagement PM",
			description: "Manages client engagements and ensures delivery excellence.",
			prompt:      "You are the Engagement PM. You manage client engagements and ensure successful delivery outcomes.",
		},
		{
			id:          "knowledge_steward",
			name:        "Knowledge Steward",
			description: "Maintains organizational knowledge base and documentation.",
			prompt:      "You are the Knowledge Steward. You curate and maintain the organization's knowledge repositories.",
		},
		{
			id:          "document_manager",
			name:        "Document Manager",
			description: "Manages documents, templates, and content lifecycle.",
			prompt:      "You are the Document Manager. You handle document creation, storage, and governance.",
		},
		{
			id:          "strategist",
			name:        "Strategist",
			description: "Provides strategic analysis and planning support.",
			prompt:      "You are the Strategist. You help teams develop and execute strategic initiatives.",
		},
		{
			id:          "backlog_steward",
			name:        "Backlog Steward",
			description: "Manages product and project backlogs, prioritization, and refinement.",
			prompt:      "You are the Backlog Steward. You maintain and prioritize work backlogs.",
		},
		{
			id:          "team_manager",
			name:        "Team Manager",
			description: "Manages team composition, skills, and capacity planning.",
			prompt:      "You are the Team Manager. You handle team composition and resource planning.",
		},
		{
			id:          "slack_manager",
			name:        "Slack Manager",
			description: "Manages Slack channels, integrations, and workspace organization.",
			prompt:      "You are the Slack Manager. You maintain Slack workspace organization and integrations.",
		},
		{
			id:          "initiateproject",
			name:        "Project Initializer",
			description: "Bootstraps new projects with templates, structure, and initial setup.",
			prompt:      "You are the Project Initializer. You set up new projects with best practices and templates.",
		},
		{
			id:          "meeting_processor",
			name:        "Meeting Processor",
			description: "Processes meeting recordings, generates transcripts, and extracts action items.",
			prompt:      "You are the Meeting Processor. You process meeting outputs and extract actionable insights.",
		},
		{
			id:          "comms_lead",
			name:        "Communications Lead",
			description: "Leads strategic communications and stakeholder engagement.",
			prompt:      "You are the Communications Lead. You lead strategic communications initiatives.",
		},
	}

	for _, p := range placeholders {
		agents = append(agents, &store.Agent{
			ID:             p.id,
			Name:           p.name,
			Description:    p.description,
			SystemPrompt:   p.prompt,
			Tools:          mustMarshalJSON([]interface{}{}), // Empty array for placeholders
			TrustOverrides: mustMarshalJSON(map[string]interface{}{}),
			ExamplePrompts: mustMarshalJSON([]string{
				fmt.Sprintf("Help with %s tasks", p.name),
				fmt.Sprintf("What can you do as %s?", p.name),
			}),
			IsActive:  true,
			CreatedBy: "system-seed",
		})
	}

	return agents
}

// Tool builders for the first 6 agents
func buildRouterTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "delegate_to_agent",
			"description": "Delegate a request to a specialized agent",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"agent_id": map[string]interface{}{
						"type":        "string",
						"description": "ID of the target agent (e.g., 'pmo', 'task_manager')",
					},
					"request": map[string]interface{}{
						"type":        "string",
						"description": "The request to send to the agent",
					},
				},
				"required": []string{"agent_id", "request"},
			},
			"source": "internal",
		},
		{
			"name":        "classify_intent",
			"description": "Classify user intent to determine routing strategy",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "User input to classify",
					},
				},
				"required": []string{"text"},
			},
			"source": "internal",
		},
	}
}

func buildPMOTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "list_projects",
			"description": "List all projects with optional filtering",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"status": map[string]interface{}{
						"type":        "string",
						"description": "Filter by status (green/yellow/red/all)",
					},
				},
			},
			"source": "mcp",
			"server": "pmo-server",
		},
		{
			"name":        "get_project_status",
			"description": "Get detailed status for a specific project",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type":        "string",
						"description": "Project identifier",
					},
				},
				"required": []string{"project_id"},
			},
			"source": "mcp",
			"server": "pmo-server",
		},
		{
			"name":        "update_project_status",
			"description": "Update project status and health",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type": "string",
					},
					"status": map[string]interface{}{
						"type": "string",
					},
					"notes": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"project_id", "status"},
			},
			"source": "mcp",
			"server": "pmo-server",
		},
	}
}

func buildRAIDTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "list_risks",
			"description": "List all risks with optional filtering",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type": "string",
					},
					"severity": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"source": "mcp",
			"server": "raid-server",
		},
		{
			"name":        "add_risk",
			"description": "Add a new risk to the RAID log",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project_id": map[string]interface{}{
						"type": "string",
					},
					"description": map[string]interface{}{
						"type": "string",
					},
					"impact": map[string]interface{}{
						"type": "string",
					},
					"probability": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"project_id", "description", "impact", "probability"},
			},
			"source": "mcp",
			"server": "raid-server",
		},
		{
			"name":        "update_risk",
			"description": "Update an existing risk",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"risk_id": map[string]interface{}{
						"type": "string",
					},
					"status": map[string]interface{}{
						"type": "string",
					},
					"mitigation": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"risk_id"},
			},
			"source": "mcp",
			"server": "raid-server",
		},
	}
}

func buildTaskTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "list_tasks",
			"description": "List tasks with optional filtering",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"assignee": map[string]interface{}{
						"type": "string",
					},
					"status": map[string]interface{}{
						"type": "string",
					},
				},
			},
			"source": "mcp",
			"server": "task-server",
		},
		{
			"name":        "create_task",
			"description": "Create a new task",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type": "string",
					},
					"description": map[string]interface{}{
						"type": "string",
					},
					"assignee": map[string]interface{}{
						"type": "string",
					},
					"due_date": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"title"},
			},
			"source": "mcp",
			"server": "task-server",
		},
		{
			"name":        "update_task",
			"description": "Update task status or details",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"task_id": map[string]interface{}{
						"type": "string",
					},
					"status": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"task_id"},
			},
			"source": "mcp",
			"server": "task-server",
		},
	}
}

func buildCommsTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "send_email",
			"description": "Send an email message",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"to": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"subject": map[string]interface{}{
						"type": "string",
					},
					"body": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"to", "subject", "body"},
			},
			"source": "mcp",
			"server": "email-server",
		},
		{
			"name":        "send_slack_message",
			"description": "Send a Slack message",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"channel": map[string]interface{}{
						"type": "string",
					},
					"message": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"channel", "message"},
			},
			"source": "mcp",
			"server": "slack-server",
		},
		{
			"name":        "list_channels",
			"description": "List available Slack channels",
			"parameters": map[string]interface{}{
				"type": "object",
			},
			"source": "mcp",
			"server": "slack-server",
		},
	}
}

func buildMeetingTools() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":        "schedule_meeting",
			"description": "Schedule a new meeting",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title": map[string]interface{}{
						"type": "string",
					},
					"attendees": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"start_time": map[string]interface{}{
						"type": "string",
					},
					"duration_minutes": map[string]interface{}{
						"type": "number",
					},
				},
				"required": []string{"title", "attendees", "start_time"},
			},
			"source": "mcp",
			"server": "calendar-server",
		},
		{
			"name":        "find_available_slots",
			"description": "Find time slots when all attendees are available",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"attendees": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"duration_minutes": map[string]interface{}{
						"type": "number",
					},
				},
				"required": []string{"attendees", "duration_minutes"},
			},
			"source": "mcp",
			"server": "calendar-server",
		},
		{
			"name":        "list_meetings",
			"description": "List upcoming meetings",
			"parameters": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"days_ahead": map[string]interface{}{
						"type": "number",
					},
				},
			},
			"source": "mcp",
			"server": "calendar-server",
		},
	}
}

// mustMarshalJSON marshals data to JSON or panics.
// Safe to use in initialization code where errors should not occur.
func mustMarshalJSON(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return data
}
