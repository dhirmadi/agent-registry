import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { DashboardPage } from './DashboardPage';
import type { DiscoveryResponse } from '../types';

const mockDiscovery: DiscoveryResponse = {
  agents: [
    {
      id: '1',
      name: 'Agent A',
      description: 'Test agent',
      system_prompt: '',
      tools: [],
      trust_overrides: {},
      capabilities: [],
      example_prompts: [],
      required_connections: [],
      is_active: true,
      version: 1,
      created_by: 'admin',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
    {
      id: '2',
      name: 'Agent B',
      description: 'Another agent',
      system_prompt: '',
      tools: [],
      trust_overrides: {},
      capabilities: [],
      example_prompts: [],
      required_connections: [],
      is_active: false,
      version: 1,
      created_by: 'admin',
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
  ],
  mcp_servers: [
    {
      id: '1',
      label: 'MCP 1',
      endpoint: 'http://localhost:3000',
      auth_type: 'none',
      health_endpoint: '/health',
      circuit_breaker: { fail_threshold: 5, open_duration_s: 30 },
      discovery_interval: '30s',
      is_enabled: true,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
    },
  ],
  trust_defaults: [],
  model_config: {},
  fetched_at: '2026-01-01T00:00:00Z',
};

function mockFetchSuccess() {
  vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: mockDiscovery,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchError() {
  vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
    ok: false,
    status: 500,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'INTERNAL', message: 'Server error' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-2' },
    }),
  } as Response);
}

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('shows a loading spinner initially', () => {
    mockFetchSuccess();
    render(<DashboardPage />);
    expect(screen.getByTestId('dashboard-loading')).toBeInTheDocument();
  });

  it('renders summary cards with correct counts after loading', async () => {
    mockFetchSuccess();
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('dashboard-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Dashboard')).toBeInTheDocument();

    // 2 agents, 1 MCP server
    const agentsCard = screen.getByTestId('card-agents');
    expect(agentsCard).toHaveTextContent('2');

    const mcpCard = screen.getByTestId('card-mcp-servers');
    expect(mcpCard).toHaveTextContent('1');
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByTestId('dashboard-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });
});
