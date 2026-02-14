import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { AgentsPage } from './AgentsPage';
import type { Agent } from '../types';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockAgent1: Agent = {
  id: 'pmo',
  name: 'PMO Agent',
  description: 'Governance, compliance, reporting.',
  system_prompt: 'You are the PMO Agent...',
  tools: [
    { name: 'get_project_config', source: 'internal', server_label: '', description: 'Read config' },
    { name: 'git_read_file', source: 'mcp', server_label: 'mcp-git', description: 'Read a file' },
  ],
  trust_overrides: { git_read_file: 'auto' },
  capabilities: ['get_project_config'],
  example_prompts: ['Run a health check'],
  required_connections: ['slack-mcp'],
  is_active: true,
  version: 3,
  created_by: 'system',
  created_at: '2026-01-15T10:00:00Z',
  updated_at: '2026-02-10T14:30:00Z',
};

const mockAgent2: Agent = {
  id: 'knowledge',
  name: 'Knowledge Steward',
  description: 'Manages glossary and documentation.',
  system_prompt: 'You are the Knowledge Steward...',
  tools: [],
  trust_overrides: {},
  capabilities: [],
  example_prompts: [],
  required_connections: [],
  is_active: false,
  version: 1,
  created_by: 'admin',
  created_at: '2026-01-20T08:00:00Z',
  updated_at: '2026-01-20T08:00:00Z',
};

let fetchMock: Mock;

function mockFetchAgents(agents: Agent[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: agents, total },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchError() {
  fetchMock.mockResolvedValueOnce({
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

function mockDeleteSuccess() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 204,
  } as Response);
}

function mockToggleActiveSuccess(agent: Agent) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { ...agent, is_active: !agent.is_active },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-3' },
    }),
  } as Response);
}

function mockCreateSuccess(agent: Agent) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 201,
    json: async () => ({
      success: true,
      data: agent,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-4' },
    }),
  } as Response);
}

function renderAgentsPage() {
  return render(
    <MemoryRouter initialEntries={['/agents']}>
      <AgentsPage />
    </MemoryRouter>,
  );
}

describe('AgentsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockNavigate.mockReset();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows a loading spinner initially', () => {
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();
    expect(screen.getByTestId('agents-loading')).toBeInTheDocument();
  });

  it('renders agent table with correct data after loading', async () => {
    mockFetchAgents([mockAgent1, mockAgent2], 2);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Agents')).toBeInTheDocument();
    expect(screen.getByText('PMO Agent')).toBeInTheDocument();
    expect(screen.getByText('Knowledge Steward')).toBeInTheDocument();
    expect(screen.getByText('pmo')).toBeInTheDocument();
    expect(screen.getByText('knowledge')).toBeInTheDocument();
  });

  it('displays active/inactive status labels', async () => {
    mockFetchAgents([mockAgent1, mockAgent2], 2);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    const activeLabels = screen.getAllByText('Active');
    const inactiveLabels = screen.getAllByText('Inactive');
    expect(activeLabels.length).toBeGreaterThanOrEqual(1);
    expect(inactiveLabels.length).toBeGreaterThanOrEqual(1);
  });

  it('shows tool count for each agent', async () => {
    mockFetchAgents([mockAgent1, mockAgent2], 2);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    // mockAgent1 has 2 tools, mockAgent2 has 0
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('0')).toBeInTheDocument();
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.getByTestId('agents-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('navigates to agent detail on row click', async () => {
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    await userEvent.setup().click(screen.getByText('PMO Agent'));

    expect(mockNavigate).toHaveBeenCalledWith('/agents/pmo');
  });

  it('shows Create Agent button for admin', async () => {
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByRole('button', { name: /create agent/i })).toBeInTheDocument();
  });

  it('hides Create Agent button for viewer', async () => {
    mockUser = { role: 'viewer' };
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByRole('button', { name: /create agent/i })).not.toBeInTheDocument();
  });

  it('hides kebab actions for viewer', async () => {
    mockUser = { role: 'viewer' };
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('agent-actions-pmo')).not.toBeInTheDocument();
  });

  it('opens create modal and submits new agent', async () => {
    mockFetchAgents([], 0);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('button', { name: /create agent/i }));

    // Modal should be open
    expect(screen.getByText('Create New Agent')).toBeInTheDocument();

    // Fill form fields using input IDs
    await ue.type(document.getElementById('create-agent-id')!, 'test-agent');
    await ue.type(document.getElementById('create-agent-name')!, 'Test Agent');
    await ue.type(document.getElementById('create-agent-desc')!, 'A test agent');

    // Submit
    const newAgent: Agent = {
      ...mockAgent1,
      id: 'test-agent',
      name: 'Test Agent',
      description: 'A test agent',
      version: 1,
    };
    mockCreateSuccess(newAgent);
    mockFetchAgents([newAgent], 1);

    await ue.click(screen.getByTestId('create-agent-submit'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
      expect(postCall![0]).toBe('/api/v1/agents');
    });
  });

  it('filters agents by name using search input', async () => {
    mockFetchAgents([mockAgent1, mockAgent2], 2);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/filter by name/i);
    await userEvent.setup().type(searchInput, 'PMO');

    expect(screen.getByText('PMO Agent')).toBeInTheDocument();
    expect(screen.queryByText('Knowledge Steward')).not.toBeInTheDocument();
  });

  it('shows empty state when no agents exist', async () => {
    mockFetchAgents([], 0);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText(/no agents found/i)).toBeInTheDocument();
  });

  it('calls DELETE API and removes agent on delete action', async () => {
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Open kebab menu
    const kebab = screen.getByTestId('agent-actions-pmo').querySelector('button')!;
    await ue.click(kebab);

    // Click delete in dropdown
    const deleteItem = await screen.findByRole('menuitem', { name: /delete/i });
    await ue.click(deleteItem);

    // ConfirmDialog should appear
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();

    // Confirm deletion
    mockDeleteSuccess();
    mockFetchAgents([], 0);
    await ue.click(screen.getByTestId('confirm-button'));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
      expect(deleteCall![0]).toContain('/api/v1/agents/pmo');
    });
  });

  it('toggles agent active status via PATCH', async () => {
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Open kebab menu
    const kebab = screen.getByTestId('agent-actions-pmo').querySelector('button')!;
    await ue.click(kebab);

    // Click toggle in dropdown
    mockToggleActiveSuccess(mockAgent1);
    mockFetchAgents([{ ...mockAgent1, is_active: false }], 1);
    const deactivateItem = await screen.findByRole('menuitem', { name: /deactivate/i });
    await ue.click(deactivateItem);

    await waitFor(() => {
      const patchCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PATCH',
      );
      expect(patchCall).toBeDefined();
    });
  });

  it('fetches with active_only=false to show all agents', async () => {
    mockFetchAgents([mockAgent1], 1);
    renderAgentsPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agents-loading')).not.toBeInTheDocument();
    });

    const firstCall = fetchMock.mock.calls[0];
    expect(firstCall[0]).toContain('active_only=false');
  });
});
