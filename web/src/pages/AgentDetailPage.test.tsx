import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AgentDetailPage } from './AgentDetailPage';
import type { Agent, AgentVersion, Prompt } from '../types';

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

const mockAddToast = vi.fn();
vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: mockAddToast }),
}));

const mockAgent: Agent = {
  id: 'pmo',
  name: 'PMO Agent',
  description: 'Governance, compliance, reporting.',
  system_prompt: 'You are the PMO Agent for workspace "{{workspace_name}}".',
  tools: [
    { name: 'get_project_config', source: 'internal', server_label: '', description: 'Read config' },
    { name: 'git_read_file', source: 'mcp', server_label: 'mcp-git', description: 'Read a file' },
  ],
  trust_overrides: { git_read_file: 'auto' },
  capabilities: ['get_project_config'],
  example_prompts: ['Run a health check', 'Update the changelog'],
  required_connections: ['slack-mcp'],
  is_active: true,
  version: 3,
  created_by: 'system',
  created_at: '2026-01-15T10:00:00Z',
  updated_at: '2026-02-10T14:30:00Z',
};

const mockVersions: AgentVersion[] = [
  {
    id: 'v3-uuid',
    agent_id: 'pmo',
    version: 3,
    name: 'PMO Agent',
    description: 'Governance, compliance, reporting.',
    system_prompt: 'You are the PMO Agent...',
    tools: mockAgent.tools,
    trust_overrides: { git_read_file: 'auto' },
    example_prompts: ['Run a health check'],
    is_active: true,
    created_by: 'admin',
    created_at: '2026-02-10T14:30:00Z',
  },
  {
    id: 'v2-uuid',
    agent_id: 'pmo',
    version: 2,
    name: 'PMO Agent',
    description: 'Old description.',
    system_prompt: 'You are the PMO Agent (v2)...',
    tools: [],
    trust_overrides: {},
    example_prompts: [],
    is_active: true,
    created_by: 'admin',
    created_at: '2026-02-08T10:00:00Z',
  },
];

const mockPrompts: Prompt[] = [
  {
    id: 'prompt-1',
    agent_id: 'pmo',
    version: 2,
    system_prompt: 'You are the PMO Agent v2...',
    template_vars: {},
    mode: 'toolcalling_auto',
    is_active: true,
    created_by: 'admin',
    created_at: '2026-02-10T14:30:00Z',
  },
  {
    id: 'prompt-2',
    agent_id: 'pmo',
    version: 1,
    system_prompt: 'You are the PMO Agent v1...',
    template_vars: {},
    mode: 'rag_readonly',
    is_active: false,
    created_by: 'admin',
    created_at: '2026-01-15T10:00:00Z',
  },
];

let fetchMock: Mock;

function mockFetchAgent() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: mockAgent,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchVersions() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { versions: mockVersions, total: 2 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-2' },
    }),
  } as Response);
}

function mockFetchPrompts() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { prompts: mockPrompts, total: mockPrompts.length },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-5' },
    }),
  } as Response);
}

function mockFetchAgentError() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 404,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'NOT_FOUND', message: "agent 'nonexistent' not found" },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-3' },
    }),
  } as Response);
}

function mockSaveSuccess() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { ...mockAgent, version: 4, updated_at: '2026-02-14T12:00:00Z' },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-save' },
    }),
  } as Response);
}

function mockSaveConflict() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 409,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'CONFLICT', message: 'resource was modified by another client' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-conflict' },
    }),
  } as Response);
}

function mockAllFetches() {
  mockFetchAgent();
  mockFetchVersions();
  mockFetchPrompts();
}

function renderAgentDetailPage(agentId = 'pmo') {
  return render(
    <MemoryRouter initialEntries={[`/agents/${agentId}`]}>
      <Routes>
        <Route path="/agents/:agentId" element={<AgentDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('AgentDetailPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockNavigate.mockReset();
    mockAddToast.mockReset();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows a loading spinner initially', () => {
    mockAllFetches();
    renderAgentDetailPage();
    expect(screen.getByTestId('agent-detail-loading')).toBeInTheDocument();
  });

  it('renders agent name and details after loading', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('PMO Agent')).toBeInTheDocument();
    expect(screen.getByText('pmo')).toBeInTheDocument();
  });

  it('shows error when agent is not found', async () => {
    mockFetchAgentError();
    // versions and prompts fail too
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => ({
        success: false,
        data: null,
        error: { code: 'NOT_FOUND', message: 'not found' },
        meta: { timestamp: new Date().toISOString(), request_id: 'req-4' },
      }),
    } as Response);
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => ({
        success: false,
        data: null,
        error: { code: 'NOT_FOUND', message: 'not found' },
        meta: { timestamp: new Date().toISOString(), request_id: 'req-4b' },
      }),
    } as Response);
    renderAgentDetailPage('nonexistent');

    await waitFor(() => {
      expect(screen.getByTestId('agent-detail-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/not found/i)).toBeInTheDocument();
  });

  it('displays General tab with editable form fields', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    // General tab should be active by default and show form fields
    expect(screen.getByLabelText(/^name$/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/description/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/example prompts/i)).toBeInTheDocument();
  });

  it('saves General tab form via PUT with If-Match', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Modify name field
    const nameInput = screen.getByLabelText(/^name$/i);
    await ue.clear(nameInput);
    await ue.type(nameInput, 'Updated PMO Agent');

    // Click save
    mockSaveSuccess();
    // After save, page re-fetches
    mockFetchAgent();
    mockFetchVersions();
    mockFetchPrompts();
    await ue.click(screen.getByTestId('save-general'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      expect(putCall![0]).toBe('/api/v1/agents/pmo');
      // Should include If-Match header
      const headers = new Headers((putCall![1] as RequestInit).headers);
      expect(headers.get('If-Match')).toBe(mockAgent.updated_at);
    });
  });

  it('shows conflict dialog on 409 response', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Modify name field
    const nameInput = screen.getByLabelText(/^name$/i);
    await ue.clear(nameInput);
    await ue.type(nameInput, 'Conflicting name');

    // Click save with conflict
    mockSaveConflict();
    await ue.click(screen.getByTestId('save-general'));

    await waitFor(() => {
      expect(screen.getByText(/modified by another/i)).toBeInTheDocument();
    });
  });

  it('switches to Tools tab and shows tools', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /tools/i }));

    expect(screen.getByText('get_project_config')).toBeInTheDocument();
    expect(screen.getAllByText('git_read_file').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('internal')).toBeInTheDocument();
    expect(screen.getByText('mcp-git')).toBeInTheDocument();
  });

  it('switches to Trust Overrides tab and shows overrides', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /trust overrides/i }));

    expect(screen.getAllByText('git_read_file').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('auto')).toBeInTheDocument();
  });

  it('switches to System Prompt tab and shows editable textarea', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /system prompt/i }));

    const textarea = screen.getByTestId('system-prompt-editor');
    expect(textarea).toBeInTheDocument();
    expect(textarea).toHaveValue(mockAgent.system_prompt);
  });

  it('switches to Prompt Versions tab and shows prompts', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /prompt versions/i }));

    expect(screen.getByText('toolcalling_auto')).toBeInTheDocument();
    expect(screen.getByText('rag_readonly')).toBeInTheDocument();
  });

  it('switches to Version History tab and shows versions with rollback', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /version history/i }));

    // v3 appears in header label AND timeline — check at least 1
    expect(screen.getAllByText(/v3/).length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/v2/).length).toBeGreaterThanOrEqual(1);
    // Version 2 (non-current) should have rollback button
    expect(screen.getByTestId('rollback-2')).toBeInTheDocument();
  });

  it('shows active badge and version number', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.getAllByText('Active').length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/v3/).length).toBeGreaterThanOrEqual(1);
  });

  it('provides Back to Agents navigation', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    const backButton = screen.getByRole('button', { name: /back to agents/i });
    await ue.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith('/agents');
  });

  it('fetches agent data from correct endpoint', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/agents/pmo');
  });

  it('fetches version history from correct endpoint', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/agents/pmo/versions');
  });

  it('fetches prompts from correct endpoint', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    expect(fetchMock.mock.calls[2][0]).toBe('/api/v1/agents/pmo/prompts');
  });

  it('calls rollback API when rollback button is clicked', async () => {
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /version history/i }));

    // Mock rollback response
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockAgent, version: 4 },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-rollback' },
      }),
    } as Response);
    // Mock re-fetches after rollback
    mockAllFetches();

    await ue.click(screen.getByTestId('rollback-2'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => {
          const url = call[0] as string;
          return url.includes('/rollback') && (call[1] as RequestInit)?.method === 'POST';
        },
      );
      expect(postCall).toBeDefined();
      expect(postCall![0]).toBe('/api/v1/agents/pmo/rollback');
      const body = JSON.parse((postCall![1] as RequestInit).body as string);
      expect(body.target_version).toBe(2);
    });
  });

  it('hides save buttons for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockAllFetches();
    renderAgentDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('agent-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('save-general')).not.toBeInTheDocument();
  });
});
