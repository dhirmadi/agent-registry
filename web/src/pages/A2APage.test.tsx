import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { A2APage } from './A2APage';
import type { A2AAgentCard } from '../types';

vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: { role: 'admin' } }),
}));

vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}));

const mockCard1: A2AAgentCard = {
  name: 'PMO Agent',
  description: 'Governance, compliance, reporting.',
  url: 'http://localhost:8080/api/v1/agents/pmo',
  version: '3',
  protocolVersion: '0.3.0',
  provider: { organization: 'Agentic Registry', url: 'http://localhost:8080' },
  capabilities: { streaming: false, pushNotifications: false },
  defaultInputModes: ['text'],
  defaultOutputModes: ['text'],
  skills: [
    {
      id: 'get_project_config',
      name: 'get_project_config',
      description: 'Read config',
      tags: ['internal'],
      examples: ['Run a health check'],
    },
  ],
  securitySchemes: { bearerAuth: { type: 'http', scheme: 'bearer' } },
  security: [{ bearerAuth: [] }],
};

const mockCard2: A2AAgentCard = {
  name: 'Knowledge Steward',
  description: 'Manages glossary and documentation.',
  url: 'http://localhost:8080/api/v1/agents/knowledge',
  version: '1',
  protocolVersion: '0.3.0',
  provider: { organization: 'Agentic Registry', url: 'http://localhost:8080' },
  capabilities: { streaming: false, pushNotifications: false },
  defaultInputModes: ['text'],
  defaultOutputModes: ['text'],
  skills: [
    {
      id: 'knowledge',
      name: 'Knowledge Steward',
      description: 'Manages glossary and documentation.',
      tags: [],
      examples: [],
    },
  ],
  securitySchemes: { bearerAuth: { type: 'http', scheme: 'bearer' } },
  security: [{ bearerAuth: [] }],
};

const mockWellKnownCard: A2AAgentCard = {
  name: 'Agentic Registry',
  description: 'A registry of AI agents and their configurations',
  url: 'http://localhost:8080',
  version: '1.0.0',
  protocolVersion: '0.3.0',
  provider: { organization: 'Agentic Registry', url: 'http://localhost:8080' },
  capabilities: { streaming: false, pushNotifications: false },
  defaultInputModes: ['text'],
  defaultOutputModes: ['text'],
  skills: [
    { id: 'pmo', name: 'PMO Agent', description: 'Governance', tags: ['agent'], examples: [] },
  ],
  securitySchemes: { bearerAuth: { type: 'http', scheme: 'bearer' } },
  security: [{ bearerAuth: [] }],
};

let fetchMock: Mock;

function mockFetchWellKnown(card: A2AAgentCard) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    headers: new Headers({
      'Content-Type': 'application/json',
      ETag: '"2026-02-10T14:30:00Z"',
    }),
    json: async () => card,
  } as Response);
}

function mockFetchIndex(cards: A2AAgentCard[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => ({
      success: true,
      data: { agent_cards: cards, total },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-idx' },
    }),
  } as Response);
}

function mockFetchWellKnownError() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 500,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => ({ error: 'internal server error' }),
  } as Response);
}

function mockFetchIndexError() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 500,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'INTERNAL', message: 'Server error' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err' },
    }),
  } as Response);
}

describe('A2APage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
  });

  it('shows loading spinners initially', () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([mockCard1], 1);
    render(<A2APage />);

    expect(screen.getByTestId('wellknown-loading')).toBeInTheDocument();
    expect(screen.getByTestId('index-loading')).toBeInTheDocument();
  });

  it('renders the page title', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([mockCard1], 1);
    render(<A2APage />);

    expect(screen.getByText('A2A Agent Cards')).toBeInTheDocument();
  });

  it('displays the well-known card after loading', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([mockCard1], 1);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('wellknown-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('wellknown-card')).toBeInTheDocument();
    expect(screen.getByText(/Agentic Registry/)).toBeInTheDocument();
    // ETag should be displayed
    expect(screen.getByText(/"2026-02-10T14:30:00Z"/)).toBeInTheDocument();
  });

  it('shows error when well-known fetch fails', async () => {
    mockFetchWellKnownError();
    mockFetchIndex([mockCard1], 1);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.getByTestId('wellknown-error')).toBeInTheDocument();
    });
  });

  it('displays agent card index with cards after loading', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([mockCard1, mockCard2], 2);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('index-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('index-table')).toBeInTheDocument();
    expect(screen.getByText('PMO Agent')).toBeInTheDocument();
    expect(screen.getByText('Knowledge Steward')).toBeInTheDocument();
  });

  it('shows error when index fetch fails', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndexError();
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.getByTestId('index-error')).toBeInTheDocument();
    });
  });

  it('searches agent cards with q parameter', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([mockCard1, mockCard2], 2);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('index-loading')).not.toBeInTheDocument();
    });

    // Mock the search fetch
    mockFetchIndex([mockCard1], 1);

    const ue = userEvent.setup();
    const searchInput = screen.getByPlaceholderText(/search agents/i);
    await ue.type(searchInput, 'PMO');

    // Wait for debounced search to trigger
    await waitFor(() => {
      const searchCall = fetchMock.mock.calls.find(
        (call: unknown[]) => typeof call[0] === 'string' && (call[0] as string).includes('q=PMO'),
      );
      expect(searchCall).toBeDefined();
    });
  });

  it('expands a card row to show full JSON', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([mockCard1], 1);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('index-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    const expandBtn = screen.getByTestId('expand-card-0');
    await ue.click(expandBtn);

    await waitFor(() => {
      expect(screen.getByTestId('card-json-0')).toBeInTheDocument();
    });
  });

  it('shows the external registry config section', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([], 0);
    render(<A2APage />);

    expect(screen.getByText('External Registry Configuration')).toBeInTheDocument();
    expect(screen.getByTestId('registry-config-section')).toBeInTheDocument();
  });

  it('shows empty state when no agent cards exist', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([], 0);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('index-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText(/no agent cards found/i)).toBeInTheDocument();
  });

  it('paginates agent cards', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    // First load: 20 cards, total 25
    const cards = Array.from({ length: 20 }, (_, i) => ({
      ...mockCard1,
      name: `Agent ${i}`,
      url: `http://localhost:8080/api/v1/agents/agent-${i}`,
    }));
    mockFetchIndex(cards, 25);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('index-loading')).not.toBeInTheDocument();
    });

    // Should see pagination info
    expect(screen.getByTestId('pagination-info')).toBeInTheDocument();

    // Click "Next" page
    mockFetchIndex([mockCard2], 25);
    const ue = userEvent.setup();
    const nextBtn = screen.getByTestId('next-page');
    await ue.click(nextBtn);

    await waitFor(() => {
      const pageCall = fetchMock.mock.calls.find(
        (call: unknown[]) => typeof call[0] === 'string' && (call[0] as string).includes('offset=20'),
      );
      expect(pageCall).toBeDefined();
    });
  });

  it('has a Copy URL button for well-known endpoint', async () => {
    mockFetchWellKnown(mockWellKnownCard);
    mockFetchIndex([], 0);
    render(<A2APage />);

    await waitFor(() => {
      expect(screen.queryByTestId('wellknown-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('copy-wellknown-url')).toBeInTheDocument();
  });
});
