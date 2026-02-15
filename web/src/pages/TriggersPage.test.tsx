import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { TriggersPage } from './TriggersPage';
import type { TriggerRule } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockAddToast = vi.fn();
vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: mockAddToast }),
}));

const mockTriggers: TriggerRule[] = [
  {
    id: 'trig-1',
    workspace_id: 'ws-1',
    name: 'On new message',
    event_type: 'message_received',
    condition: { channel: '#general' },
    agent_id: 'pmo',
    prompt_template: 'Handle message: {{message}}',
    enabled: true,
    rate_limit_per_hour: 10,
    schedule: '',
    run_as_user_id: null,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'trig-2',
    workspace_id: 'ws-1',
    name: 'Daily report',
    event_type: 'schedule',
    condition: {},
    agent_id: 'knowledge',
    prompt_template: '',
    enabled: false,
    rate_limit_per_hour: 1,
    schedule: '0 9 * * *',
    run_as_user_id: null,
    created_at: '2026-01-02T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  },
];

let fetchMock: Mock;

function mockFetchTriggersSuccess(triggers: TriggerRule[] = mockTriggers) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: triggers, total: triggers.length },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchTriggersError() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 500,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'INTERNAL', message: 'Server error' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err' },
    }),
  } as Response);
}

describe('TriggersPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  /** Helper: enter workspace ID and click Search to load triggers */
  async function searchWorkspace(ue: ReturnType<typeof userEvent.setup>) {
    const input = screen.getByTestId('workspace-id-input');
    await ue.type(input, 'ws-1');
    await ue.click(screen.getByTestId('search-triggers-btn'));
  }

  it('shows prompt to enter workspace ID initially', () => {
    render(<TriggersPage />);
    expect(screen.getByText(/Enter a workspace ID/)).toBeInTheDocument();
  });

  it('renders triggers table after searching workspace', async () => {
    mockFetchTriggersSuccess();
    render(<TriggersPage />);

    const ue = userEvent.setup();
    await searchWorkspace(ue);

    await waitFor(() => {
      expect(screen.queryByTestId('triggers-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Trigger Rules')).toBeInTheDocument();
    expect(screen.getByTestId('triggers-table')).toBeInTheDocument();
    expect(screen.getByText('On new message')).toBeInTheDocument();
    expect(screen.getByText('Daily report')).toBeInTheDocument();
    expect(screen.getByText('message_received')).toBeInTheDocument();
    expect(screen.getByText('schedule')).toBeInTheDocument();
    expect(screen.getByText('pmo')).toBeInTheDocument();
    expect(screen.getByText('0 9 * * *')).toBeInTheDocument();
  });

  it('shows error when API fails', async () => {
    mockFetchTriggersError();
    render(<TriggersPage />);

    const ue = userEvent.setup();
    await searchWorkspace(ue);

    await waitFor(() => {
      expect(screen.getByTestId('triggers-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('toggles trigger enabled status via PUT', async () => {
    mockFetchTriggersSuccess();
    render(<TriggersPage />);

    const ue = userEvent.setup();
    await searchWorkspace(ue);

    await waitFor(() => {
      expect(screen.queryByTestId('triggers-loading')).not.toBeInTheDocument();
    });

    // Mock the PUT call and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockTriggers[0], enabled: false },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-put' },
      }),
    } as Response);
    mockFetchTriggersSuccess();

    await ue.click(screen.getByTestId('toggle-trigger-trig-1'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      expect((putCall![0] as string)).toContain('/api/v1/workspaces/ws-1/trigger-rules/trig-1');
    });
  });

  it('opens create modal and submits new trigger', async () => {
    mockFetchTriggersSuccess();
    render(<TriggersPage />);

    const ue = userEvent.setup();
    await searchWorkspace(ue);

    await waitFor(() => {
      expect(screen.queryByTestId('triggers-loading')).not.toBeInTheDocument();
    });

    await ue.click(screen.getByTestId('create-trigger-btn'));

    expect(screen.getByText('Create Trigger Rule')).toBeInTheDocument();

    await ue.type(screen.getByPlaceholderText('e.g., On new message'), 'New trigger');
    await ue.type(screen.getByPlaceholderText('e.g., pmo'), 'test-agent');

    // Mock POST and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: { id: 'trig-new', name: 'New trigger' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-create' },
      }),
    } as Response);
    mockFetchTriggersSuccess();

    await ue.click(screen.getByTestId('submit-trigger-btn'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
      expect((postCall![0] as string)).toContain('/api/v1/workspaces/ws-1/trigger-rules');
    });
  });

  it('hides create button when no workspace entered', () => {
    render(<TriggersPage />);
    expect(screen.queryByTestId('create-trigger-btn')).not.toBeInTheDocument();
  });

  it('deletes a trigger with confirmation', async () => {
    mockFetchTriggersSuccess();
    render(<TriggersPage />);

    const ue = userEvent.setup();
    await searchWorkspace(ue);

    await waitFor(() => {
      expect(screen.queryByTestId('triggers-loading')).not.toBeInTheDocument();
    });

    await ue.click(screen.getByTestId('delete-trigger-trig-1'));

    expect(screen.getByText(/Are you sure you want to delete trigger/)).toBeInTheDocument();

    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 204,
    } as Response);
    mockFetchTriggersSuccess();

    await ue.click(screen.getByTestId('confirm-button'));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
      expect((deleteCall![0] as string)).toContain('/api/v1/workspaces/ws-1/trigger-rules/trig-1');
    });
  });
});
