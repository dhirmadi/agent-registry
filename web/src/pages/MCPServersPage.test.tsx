import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach, type MockInstance } from 'vitest';
import { MCPServersPage } from './MCPServersPage';
import type { MCPServer } from '../types';

type FetchSpy = MockInstance<typeof globalThis.fetch>;

const mockServers: MCPServer[] = [
  {
    id: 'srv-1',
    label: 'mcp-git',
    endpoint: 'http://mcp-git:8080/mcp',
    auth_type: 'none',
    health_endpoint: 'http://mcp-git:8080/health',
    circuit_breaker: { fail_threshold: 5, open_duration_s: 30 },
    discovery_interval: '5m',
    is_enabled: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'srv-2',
    label: 'slack-mcp',
    endpoint: 'http://slack-mcp:8080/sse',
    auth_type: 'bearer',
    health_endpoint: '',
    circuit_breaker: { fail_threshold: 3, open_duration_s: 60 },
    discovery_interval: '10m',
    is_enabled: false,
    created_at: '2026-01-02T00:00:00Z',
    updated_at: '2026-01-02T00:00:00Z',
  },
];

function mockFetchServersSuccess() {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { servers: mockServers, total: 2 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchServersError() {
  return vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
    ok: false,
    status: 500,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'INTERNAL', message: 'Failed to load servers' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err' },
    }),
  } as Response);
}

function mockCreateServerSuccess(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 201,
    json: async () => ({
      success: true,
      data: {
        id: 'srv-3',
        label: 'new-mcp',
        endpoint: 'http://new-mcp:8080/mcp',
        auth_type: 'basic',
        health_endpoint: '/health',
        circuit_breaker: { fail_threshold: 5, open_duration_s: 30 },
        discovery_interval: '5m',
        is_enabled: true,
        created_at: '2026-02-14T00:00:00Z',
        updated_at: '2026-02-14T00:00:00Z',
      },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-create' },
    }),
  } as Response);
}

function mockDeleteServerSuccess(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 204,
    json: async () => { throw new Error('no body'); },
  } as unknown as Response);
}

function mockUpdateServerSuccess(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: {
        ...mockServers[0],
        label: 'mcp-git-updated',
        updated_at: '2026-02-14T00:00:00Z',
      },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-update' },
    }),
  } as Response);
}

function mockRefetchServers(fetchSpy: FetchSpy) {
  fetchSpy.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { servers: mockServers, total: 2 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-refetch' },
    }),
  } as Response);
}

describe('MCPServersPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('shows loading spinner initially', () => {
    mockFetchServersSuccess();
    render(<MCPServersPage />);
    expect(screen.getByTestId('mcp-loading')).toBeInTheDocument();
  });

  it('renders server table after loading', async () => {
    mockFetchServersSuccess();
    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('mcp-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('MCP Servers')).toBeInTheDocument();
    expect(screen.getByTestId('mcp-table')).toBeInTheDocument();

    // Verify server data is displayed
    expect(screen.getByText('mcp-git')).toBeInTheDocument();
    expect(screen.getByText('slack-mcp')).toBeInTheDocument();
    expect(screen.getByText('http://mcp-git:8080/mcp')).toBeInTheDocument();
    expect(screen.getByText('http://slack-mcp:8080/sse')).toBeInTheDocument();
  });

  it('shows error when API call fails', async () => {
    mockFetchServersError();
    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('mcp-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Failed to load servers/)).toBeInTheDocument();
  });

  it('shows empty state when no servers exist', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { servers: [], total: 0 },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-empty' },
      }),
    } as Response);

    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('mcp-empty')).toBeInTheDocument();
    });
  });

  it('opens create modal and submits new server', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchServersSuccess();
    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('mcp-loading')).not.toBeInTheDocument();
    });

    // Click create button
    const createBtn = screen.getByTestId('create-server-btn');
    await user.click(createBtn);

    // Modal should appear
    await waitFor(() => {
      expect(screen.getByTestId('server-modal')).toBeInTheDocument();
    });

    // Fill in the form
    const labelInput = screen.getByLabelText('Label');
    await user.type(labelInput, 'new-mcp');

    const endpointInput = screen.getByLabelText('Endpoint');
    await user.type(endpointInput, 'http://new-mcp:8080/mcp');

    // Submit
    mockCreateServerSuccess(fetchSpy);
    mockRefetchServers(fetchSpy);

    const saveBtn = screen.getByTestId('server-modal-save');
    await user.click(saveBtn);

    // Verify POST was called
    await waitFor(() => {
      const calls = fetchSpy.mock.calls;
      const postCall = calls.find(
        (c) =>
          typeof c[0] === 'string' &&
          c[0].includes('/mcp-servers') &&
          (c[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
    });
  });

  it('opens edit modal with server data pre-filled', async () => {
    const user = userEvent.setup();
    mockFetchServersSuccess();
    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('mcp-table')).toBeInTheDocument();
    });

    // Click edit on the first server row
    const rows = screen.getAllByTestId(/^server-row-/);
    const editBtn = within(rows[0]).getByTestId('edit-server-btn');
    await user.click(editBtn);

    await waitFor(() => {
      expect(screen.getByTestId('server-modal')).toBeInTheDocument();
    });

    // Verify the label is pre-filled
    const labelInput = screen.getByLabelText('Label') as HTMLInputElement;
    expect(labelInput.value).toBe('mcp-git');
  });

  it('deletes a server after confirmation', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchServersSuccess();
    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('mcp-table')).toBeInTheDocument();
    });

    // Click delete on the first server row
    const rows = screen.getAllByTestId(/^server-row-/);
    const deleteBtn = within(rows[0]).getByTestId('delete-server-btn');
    await user.click(deleteBtn);

    // Confirmation should appear
    await waitFor(() => {
      expect(screen.getByTestId('delete-confirm')).toBeInTheDocument();
    });

    mockDeleteServerSuccess(fetchSpy);
    mockRefetchServers(fetchSpy);

    const confirmBtn = screen.getByTestId('delete-confirm-btn');
    await user.click(confirmBtn);

    // Verify DELETE was called
    await waitFor(() => {
      const calls = fetchSpy.mock.calls;
      const deleteCall = calls.find(
        (c) =>
          typeof c[0] === 'string' &&
          c[0].includes('/mcp-servers/srv-1') &&
          (c[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
    });
  });

  it('updates a server with If-Match header', async () => {
    const user = userEvent.setup();
    const fetchSpy = mockFetchServersSuccess();
    render(<MCPServersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('mcp-table')).toBeInTheDocument();
    });

    // Click edit on the first server
    const rows = screen.getAllByTestId(/^server-row-/);
    const editBtn = within(rows[0]).getByTestId('edit-server-btn');
    await user.click(editBtn);

    await waitFor(() => {
      expect(screen.getByTestId('server-modal')).toBeInTheDocument();
    });

    // Change the label
    const labelInput = screen.getByLabelText('Label');
    await user.clear(labelInput);
    await user.type(labelInput, 'mcp-git-updated');

    mockUpdateServerSuccess(fetchSpy);
    mockRefetchServers(fetchSpy);

    const saveBtn = screen.getByTestId('server-modal-save');
    await user.click(saveBtn);

    // Verify PUT was called with If-Match
    await waitFor(() => {
      const calls = fetchSpy.mock.calls;
      const putCall = calls.find(
        (c) =>
          typeof c[0] === 'string' &&
          c[0].includes('/mcp-servers/srv-1') &&
          (c[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      const headers = new Headers((putCall![1] as RequestInit).headers);
      expect(headers.get('If-Match')).toBe('2026-01-01T00:00:00Z');
    });
  });
});
