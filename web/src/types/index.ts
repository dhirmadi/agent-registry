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

// Model Endpoints
export type ModelProvider = 'openai' | 'azure' | 'anthropic' | 'ollama' | 'custom';

export interface ModelEndpoint {
  id: string;
  slug: string;
  name: string;
  provider: ModelProvider;
  endpoint_url: string;
  is_fixed_model: boolean;
  model_name: string;
  allowed_models: string[];
  is_active: boolean;
  workspace_id: string | null;
  version: number;
  created_by: string;
  created_at: string;
  updated_at: string;
}

export interface ModelEndpointConfig {
  temperature?: number;
  max_tokens?: number;
  max_output_tokens?: number;
  top_p?: number;
  frequency_penalty?: number;
  presence_penalty?: number;
  context_window?: number;
  history_token_budget?: number;
  max_history_messages?: number;
  max_tool_rounds?: number;
  headers?: Record<string, string>;
  metadata?: Record<string, unknown>;
}

export interface ModelEndpointVersion {
  id: string;
  endpoint_id: string;
  version: number;
  config: ModelEndpointConfig;
  is_active: boolean;
  change_note: string;
  created_by: string;
  created_at: string;
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

// A2A Agent Cards
export interface A2ASkill {
  id: string;
  name: string;
  description: string;
  tags: string[];
  examples: string[];
}

export interface A2AAgentCard {
  name: string;
  description: string;
  url: string;
  version: string;
  protocolVersion: string;
  provider: {
    organization: string;
    url: string;
  };
  capabilities: {
    streaming: boolean;
    pushNotifications: boolean;
  };
  defaultInputModes: string[];
  defaultOutputModes: string[];
  skills: A2ASkill[];
  securitySchemes: Record<string, { type: string; scheme: string }>;
  security: Record<string, string[]>[];
}

export interface A2AIndexResponse {
  agent_cards: A2AAgentCard[];
  total: number;
}

// Discovery
export interface DiscoveryResponse {
  agents: Agent[];
  mcp_servers: MCPServer[];
  trust_defaults: TrustDefault[];
  model_config: ModelConfig | Record<string, never>;
  model_endpoints: ModelEndpoint[];
  fetched_at: string;
}
