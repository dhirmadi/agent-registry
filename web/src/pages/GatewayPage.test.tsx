import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { GatewayPage } from './GatewayPage';
import type { AuditEntry } from '../types';

vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: { role: 'admin' } }),
}));

vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: vi.fn() }),
}));

const mockAuditEntry1: AuditEntry = {
  id: 1,
  actor: 'admin',
  actor_id: 'uid-1',
  action: 'gateway_tool_call',
  resource_type: 'mcp_tool',
  resource_id: 'github/create_issue',
  details: { outcome: 'success', latency_ms: 142, server_label: 'github', tool_name: 'create_issue' },
  ip_address: '127.0.0.1',
  created_at: '2026-02-15T12:00:00Z',
};

const mockAuditEntry2: AuditEntry = {
  id: 2,
  actor: 'admin',
  actor_id: 'uid-1',
  action: 'gateway_tool_call',
  resource_type: 'mcp_tool',
  resource_id: 'slack/send_message',
  details: { outcome: 'trust_denied', latency_ms: 0, server_label: 'slack', tool_name: 'send_message' },
  ip_address: '127.0.0.1',
  created_at: '2026-02-15T11:30:00Z',
};

let fetchMock: Mock;

function mockFetchTools(servers: { label: string; endpoint: string }[]) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => ({
      success: true,
      data: { servers },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-tools' },
    }),
  } as Response);
}

function mockFetchAudit(entries: AuditEntry[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => ({
      success: true,
      data: { items: entries, total, offset: 0, limit: 20 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-audit' },
    }),
  } as Response);
}

function mockFetchError() {
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

describe('GatewayPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
  });

  it('renders the page title', () => {
    mockFetchTools([]);
    mockFetchAudit([], 0);
    render(<GatewayPage />);
    expect(screen.getByText('Gateway')).toBeInTheDocument();
  });

  it('shows loading spinners initially', () => {
    mockFetchTools([]);
    mockFetchAudit([], 0);
    render(<GatewayPage />);
    expect(screen.getByTestId('tools-loading')).toBeInTheDocument();
    expect(screen.getByTestId('audit-loading')).toBeInTheDocument();
  });

  it('displays server table after loading', async () => {
    mockFetchTools([
      { label: 'github', endpoint: 'https://mcp.github.com' },
      { label: 'slack', endpoint: 'https://mcp.slack.com' },
    ]);
    mockFetchAudit([], 0);
    render(<GatewayPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('tools-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('tools-table')).toBeInTheDocument();
    expect(screen.getByText('github')).toBeInTheDocument();
    expect(screen.getByText('https://mcp.github.com')).toBeInTheDocument();
    expect(screen.getByText('slack')).toBeInTheDocument();
    expect(screen.getByText('https://mcp.slack.com')).toBeInTheDocument();
  });

  it('shows empty state when no servers available', async () => {
    mockFetchTools([]);
    mockFetchAudit([], 0);
    render(<GatewayPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('tools-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('tools-empty')).toBeInTheDocument();
  });

  it('shows error when tools fetch fails', async () => {
    mockFetchError();
    mockFetchAudit([], 0);
    render(<GatewayPage />);

    await waitFor(() => {
      expect(screen.getByTestId('tools-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('displays audit log table after loading', async () => {
    mockFetchTools([]);
    mockFetchAudit([mockAuditEntry1, mockAuditEntry2], 2);
    render(<GatewayPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('audit-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('audit-table')).toBeInTheDocument();
    expect(screen.getByText('github/create_issue')).toBeInTheDocument();
    expect(screen.getByText('slack/send_message')).toBeInTheDocument();
    expect(screen.getByText('142ms')).toBeInTheDocument();
  });

  it('shows empty state when no audit entries', async () => {
    mockFetchTools([]);
    mockFetchAudit([], 0);
    render(<GatewayPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('audit-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('audit-empty')).toBeInTheDocument();
  });

  it('shows error when audit fetch fails', async () => {
    mockFetchTools([]);
    mockFetchError();
    render(<GatewayPage />);

    await waitFor(() => {
      expect(screen.getByTestId('audit-error')).toBeInTheDocument();
    });
  });

  it('fetches from correct API endpoints', async () => {
    mockFetchTools([]);
    mockFetchAudit([], 0);
    render(<GatewayPage />);

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    const calls = fetchMock.mock.calls.map((c: unknown[]) => c[0] as string);
    expect(calls).toContain('/mcp/v1/tools');
    const auditCall = calls.find((c) => c.includes('/api/v1/audit-log'));
    expect(auditCall).toContain('action=gateway_tool_call');
    expect(auditCall).toContain('limit=20');
  });
});
