import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach, type MockInstance } from 'vitest';
import { PromptsPage } from './PromptsPage';
import type { Agent, Prompt } from '../types';

type FetchSpy = MockInstance<typeof globalThis.fetch>;

const mockAgents: Agent[] = [
  {
    id: 'agent-1',
    name: 'PMO Agent',
    description: 'Governance agent',
    system_prompt: 'You are PMO',
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
    id: 'agent-2',
    name: 'Dev Agent',
    description: 'Development agent',
    system_prompt: 'You are Dev',
    tools: [],
    trust_overrides: {},
    capabilities: [],
    example_prompts: [],
    required_connections: [],
    is_active: true,
    version: 2,
    created_by: 'admin',
    created_at: '2026-01-02T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  },
];

const mockPrompts: Prompt[] = [
  {
    id: 'prompt-1',
    agent_id: 'agent-1',
    version: 1,
    system_prompt: 'You are the PMO Agent v1',
    template_vars: { workspace_name: '' },
    mode: 'toolcalling_safe',
    is_active: false,
    created_by: 'admin@example.com',
    created_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'prompt-2',
    agent_id: 'agent-1',
    version: 2,
    system_prompt: 'You are the PMO Agent v2',
    template_vars: { workspace_name: '', current_date: '' },
    mode: 'toolcalling_auto',
    is_active: true,
    created_by: 'admin@example.com',
    created_at: '2026-02-01T00:00:00Z',
  },
];

function mockFetchAgentsSuccess() {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { agents: mockAgents, total: 2 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchPromptsSuccess(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { prompts: mockPrompts, total: 2 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-2' },
    }),
  } as Response);
}

function mockFetchAgentsError() {
  vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
    ok: false,
    status: 500,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'INTERNAL', message: 'Failed to load agents' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err' },
    }),
  } as Response);
}

function mockFetchPromptsError(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: false,
    status: 500,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'INTERNAL', message: 'Failed to load prompts' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err2' },
    }),
  } as Response);
}

function mockCreatePromptSuccess(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 201,
    json: async () => ({
      success: true,
      data: {
        id: 'prompt-3',
        agent_id: 'agent-1',
        version: 3,
        system_prompt: 'New prompt content',
        template_vars: {},
        mode: 'rag_readonly',
        is_active: true,
        created_by: 'admin@example.com',
        created_at: '2026-02-14T00:00:00Z',
      },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-create' },
    }),
  } as Response);
}

function mockActivatePromptSuccess(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { ...mockPrompts[0], is_active: true },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-activate' },
    }),
  } as Response);
}

/** Helper: wait for agents to load, then select an agent by value using native select */
async function selectAgent(
  user: ReturnType<typeof userEvent.setup>,
  agentId: string,
) {
  await waitFor(() => {
    expect(screen.queryByTestId('prompts-loading')).not.toBeInTheDocument();
  });

  const selector = screen.getByTestId('agent-selector');
  await user.selectOptions(selector, agentId);
}

describe('PromptsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('shows loading spinner initially', () => {
    mockFetchAgentsSuccess();
    render(<PromptsPage />);
    expect(screen.getByTestId('prompts-loading')).toBeInTheDocument();
  });

  it('renders agent selector after loading', async () => {
    mockFetchAgentsSuccess();
    render(<PromptsPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('prompts-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Prompts')).toBeInTheDocument();
    expect(screen.getByTestId('agent-selector')).toBeInTheDocument();
  });

  it('shows error when agents fail to load', async () => {
    mockFetchAgentsError();
    render(<PromptsPage />);

    await waitFor(() => {
      expect(screen.getByTestId('prompts-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Failed to load agents/)).toBeInTheDocument();
  });

  it('loads and displays prompts when an agent is selected', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchAgentsSuccess();
    render(<PromptsPage />);

    mockFetchPromptsSuccess(fetchSpy);
    await selectAgent(user, 'agent-1');

    await waitFor(() => {
      expect(screen.getByTestId('prompts-table')).toBeInTheDocument();
    });

    expect(screen.getByText('toolcalling_safe')).toBeInTheDocument();
    expect(screen.getByText('toolcalling_auto')).toBeInTheDocument();
  });

  it('shows empty state when agent has no prompts', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchAgentsSuccess();
    render(<PromptsPage />);

    fetchSpy.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { prompts: [], total: 0 },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-empty' },
      }),
    } as Response);

    await selectAgent(user, 'agent-1');

    await waitFor(() => {
      expect(screen.getByTestId('prompts-empty')).toBeInTheDocument();
    });
  });

  it('shows error when prompts fail to load', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchAgentsSuccess();
    render(<PromptsPage />);

    mockFetchPromptsError(fetchSpy);
    await selectAgent(user, 'agent-1');

    await waitFor(() => {
      expect(screen.getByTestId('prompts-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Failed to load prompts/)).toBeInTheDocument();
  });

  it('opens create modal and submits new prompt', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchAgentsSuccess();
    render(<PromptsPage />);

    mockFetchPromptsSuccess(fetchSpy);
    await selectAgent(user, 'agent-1');

    await waitFor(() => {
      expect(screen.getByTestId('prompts-table')).toBeInTheDocument();
    });

    const createBtn = screen.getByTestId('create-prompt-btn');
    await user.click(createBtn);

    await waitFor(() => {
      expect(screen.getByTestId('prompt-modal')).toBeInTheDocument();
    });

    const promptInput = screen.getByLabelText('System Prompt');
    await user.clear(promptInput);
    await user.type(promptInput, 'New prompt content');

    const modeSelect = screen.getByLabelText('Mode');
    await user.selectOptions(modeSelect, 'rag_readonly');

    mockCreatePromptSuccess(fetchSpy);
    mockFetchPromptsSuccess(fetchSpy);

    const saveBtn = screen.getByTestId('prompt-modal-save');
    await user.click(saveBtn);

    await waitFor(() => {
      const calls = fetchSpy.mock.calls;
      const postCall = calls.find(
        (c) =>
          typeof c[0] === 'string' &&
          c[0].includes('/prompts') &&
          (c[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
    });
  });

  it('activates a prompt version', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchAgentsSuccess();
    render(<PromptsPage />);

    mockFetchPromptsSuccess(fetchSpy);
    await selectAgent(user, 'agent-1');

    await waitFor(() => {
      expect(screen.getByTestId('prompts-table')).toBeInTheDocument();
    });

    const rows = screen.getAllByTestId(/^prompt-row-/);
    const inactiveRow = rows.find((row) => within(row).queryByText('toolcalling_safe'));
    expect(inactiveRow).toBeDefined();

    const activateBtn = within(inactiveRow!).getByTestId('activate-prompt-btn');

    mockActivatePromptSuccess(fetchSpy);
    mockFetchPromptsSuccess(fetchSpy);

    await user.click(activateBtn);

    await waitFor(() => {
      const calls = fetchSpy.mock.calls;
      const activateCall = calls.find(
        (c) =>
          typeof c[0] === 'string' &&
          c[0].includes('/activate') &&
          (c[1] as RequestInit)?.method === 'POST',
      );
      expect(activateCall).toBeDefined();
    });
  });
});
