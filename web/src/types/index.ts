// API Envelope
export interface Envelope<T = unknown> {
  success: boolean;
  data: T;
  error: { code: string; message: string } | null;
  meta: { timestamp: string; request_id: string };
}

// Auth
export interface User {
  id: string;
  username: string;
  email: string;
  display_name: string;
  role: 'admin' | 'editor' | 'viewer';
  auth_method: 'password' | 'google' | 'both';
  is_active: boolean;
  must_change_password?: boolean;
  last_login_at: string | null;
}

export interface LoginResponse {
  user: User;
  must_change_password: boolean;
}

// Agents
export interface Agent {
  id: string;
  name: string;
  description: string;
  system_prompt: string;
  tools: AgentTool[];
  trust_overrides: Record<string, string>;
  capabilities: string[];
  example_prompts: string[];
  required_connections: string[];
  is_active: boolean;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface AgentTool {
  name: string;
  source: string;
  server_label: string;
  description: string;
}

export interface AgentVersion {
  id: string;
  agent_id: string;
  version: number;
  name: string;
  description: string;
  system_prompt: string;
  tools: AgentTool[];
  trust_overrides: Record<string, string>;
  example_prompts: string[];
  is_active: boolean;
  created_by: string;
  created_at: string;
}

// Prompts
export interface Prompt {
  id: string;
  agent_id: string;
  version: number;
  system_prompt: string;
  template_vars: Record<string, string>;
  mode: 'rag_readonly' | 'toolcalling_safe' | 'toolcalling_auto';
  is_active: boolean;
  created_by: string;
  created_at: string;
}

// MCP Servers
export interface MCPServer {
  id: string;
  label: string;
  endpoint: string;
  auth_type: 'none' | 'bearer' | 'basic';
  health_endpoint: string;
  circuit_breaker: {
    fail_threshold: number;
    open_duration_s: number;
  };
  discovery_interval: string;
  is_enabled: boolean;
  created_at: string;
  updated_at: string;
}

// Trust
export interface TrustDefault {
  id: string;
  tier: 'auto' | 'review' | 'block';
  patterns: string[];
  priority: number;
  updated_at: string;
}

export interface TrustRule {
  id: string;
  workspace_id: string;
  tool_pattern: string;
  tier: 'auto' | 'review' | 'block';
  created_by: string;
  created_at: string;
  updated_at: string;
}

// Trigger Rules
export interface TriggerRule {
  id: string;
  workspace_id: string;
  name: string;
  event_type: string;
  condition: Record<string, unknown>;
  agent_id: string;
  prompt_template: string;
  enabled: boolean;
  rate_limit_per_hour: number;
  schedule: string;
  run_as_user_id: string | null;
  created_at: string;
  updated_at: string;
}

// Model Config
export interface ModelConfig {
  id: string;
  scope: 'global' | 'workspace' | 'user';
  scope_id: string;
  default_model: string;
  temperature: number;
  max_tokens: number;
  max_tool_rounds: number;
  default_context_window: number;
  default_max_output_tokens: number;
  history_token_budget: number;
  max_history_messages: number;
  embedding_model: string;
  updated_at: string;
}

// Context Config
export interface ContextConfig {
  id: string;
  scope: 'global' | 'workspace';
  scope_id: string;
  max_total_tokens: number;
  layer_budgets: Record<string, number>;
  enabled_layers: string[];
  updated_at: string;
}

// Signal Config
export interface SignalConfig {
  id: string;
  source: string;
  poll_interval: string;
  is_enabled: boolean;
  updated_at: string;
}

// Webhooks
export interface Webhook {
  id: string;
  url: string;
  events: string[];
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

// API Keys
export interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  scopes: string[];
  is_active: boolean;
  created_at: string;
  expires_at: string | null;
  last_used_at: string | null;
}

export interface APIKeyCreateResponse {
  key: string;
  id: string;
  name: string;
  scopes: string[];
  key_prefix: string;
  created_at: string;
}

// Users (admin)
export interface UserAdmin {
  id: string;
  username: string;
  email: string;
  display_name: string;
  role: 'admin' | 'editor' | 'viewer';
  auth_method: 'password' | 'google' | 'both';
  is_active: boolean;
  must_change_pass: boolean;
  last_login_at: string | null;
  created_at: string;
  updated_at: string;
}

// Audit Log
export interface AuditEntry {
  id: number;
  actor: string;
  actor_id: string | null;
  action: string;
  resource_type: string;
  resource_id: string;
  details: Record<string, unknown>;
  ip_address: string;
  created_at: string;
}

// Paginated response
export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  offset: number;
  limit: number;
}

// Discovery
export interface DiscoveryResponse {
  agents: Agent[];
  mcp_servers: MCPServer[];
  trust_defaults: TrustDefault[];
  model_config: ModelConfig | Record<string, never>;
  context_config: ContextConfig | Record<string, never>;
  signal_config: SignalConfig[];
  fetched_at: string;
}
