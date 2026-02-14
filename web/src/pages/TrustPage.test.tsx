import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { TrustPage } from './TrustPage';
import type { TrustDefault, TrustRule } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockDefaults: TrustDefault[] = [
  {
    id: 'td-1',
    tier: 'auto',
    patterns: ['git_read_*', 'git_list_*'],
    priority: 10,
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'td-2',
    tier: 'block',
    patterns: ['rm_*'],
    priority: 100,
    updated_at: '2026-01-02T00:00:00Z',
  },
];

const mockRules: TrustRule[] = [
  {
    id: 'tr-1',
    workspace_id: 'ws-1',
    tool_pattern: 'slack_send_*',
    tier: 'review',
    created_by: 'admin',
    created_at: '2026-01-10T00:00:00Z',
    updated_at: '2026-01-10T00:00:00Z',
  },
];

let fetchMock: Mock;

function mockFetchDefaultsSuccess(defaults: TrustDefault[] = mockDefaults) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: defaults, total: defaults.length },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchDefaultsError() {
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

function mockFetchRulesSuccess(rules: TrustRule[] = mockRules) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: rules, total: rules.length },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-rules' },
    }),
  } as Response);
}

describe('TrustPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows loading spinner initially', () => {
    mockFetchDefaultsSuccess();
    render(<TrustPage />);
    expect(screen.getByTestId('defaults-loading')).toBeInTheDocument();
  });

  it('renders trust defaults table after loading', async () => {
    mockFetchDefaultsSuccess();
    render(<TrustPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('defaults-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Trust Management')).toBeInTheDocument();
    expect(screen.getByTestId('defaults-table')).toBeInTheDocument();
    expect(screen.getByText('git_read_*, git_list_*')).toBeInTheDocument();
    expect(screen.getByText('rm_*')).toBeInTheDocument();
    expect(screen.getByText('10')).toBeInTheDocument();
    expect(screen.getByText('100')).toBeInTheDocument();
  });

  it('shows error when defaults API fails', async () => {
    mockFetchDefaultsError();
    render(<TrustPage />);

    await waitFor(() => {
      expect(screen.getByTestId('defaults-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('searches and displays workspace rules', async () => {
    mockFetchDefaultsSuccess();
    render(<TrustPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('defaults-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    const wsInput = screen.getByTestId('workspace-id-input');
    await ue.type(wsInput, 'ws-1');

    mockFetchRulesSuccess();
    await ue.click(screen.getByTestId('search-rules-btn'));

    await waitFor(() => {
      expect(screen.getByTestId('rules-table')).toBeInTheDocument();
    });

    expect(screen.getByText('slack_send_*')).toBeInTheDocument();
    expect(screen.getByText('review')).toBeInTheDocument();
  });

  it('hides add buttons for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockFetchDefaultsSuccess();
    render(<TrustPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('defaults-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('add-default-btn')).not.toBeInTheDocument();
  });

  it('opens create default modal and submits', async () => {
    mockFetchDefaultsSuccess();
    render(<TrustPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('defaults-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('add-default-btn'));

    expect(screen.getByText('Add Trust Default')).toBeInTheDocument();

    await ue.type(screen.getByPlaceholderText('e.g., git_*, slack_send_message'), 'test_*');

    // Mock the POST and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: { id: 'td-new', tier: 'auto', patterns: ['test_*'], priority: 0, updated_at: '2026-02-14T00:00:00Z' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-create' },
      }),
    } as Response);
    mockFetchDefaultsSuccess();

    await ue.click(screen.getByTestId('submit-default-btn'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
      expect((postCall![0] as string)).toContain('/api/v1/trust/defaults');
    });
  });
});
