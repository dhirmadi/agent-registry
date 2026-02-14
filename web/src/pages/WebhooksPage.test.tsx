import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { WebhooksPage } from './WebhooksPage';
import type { Webhook } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockWebhook1: Webhook = {
  id: 'wh-1',
  url: 'https://example.com/hook1',
  events: ['agent.created', 'agent.updated'],
  is_active: true,
  created_at: '2026-01-15T10:00:00Z',
  updated_at: '2026-01-15T10:00:00Z',
};

const mockWebhook2: Webhook = {
  id: 'wh-2',
  url: 'https://example.com/hook2',
  events: ['mcp.created'],
  is_active: false,
  created_at: '2026-01-20T08:00:00Z',
  updated_at: '2026-01-20T08:00:00Z',
};

let fetchMock: Mock;

function mockFetchWebhooks(webhooks: Webhook[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: webhooks, total },
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

describe('WebhooksPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows a loading spinner initially', () => {
    mockFetchWebhooks([mockWebhook1], 1);
    render(<WebhooksPage />);
    expect(screen.getByTestId('webhooks-loading')).toBeInTheDocument();
  });

  it('renders webhook table with correct data after loading', async () => {
    mockFetchWebhooks([mockWebhook1, mockWebhook2], 2);
    render(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('webhooks-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Webhooks')).toBeInTheDocument();
    expect(screen.getByTestId('webhooks-table')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/hook1')).toBeInTheDocument();
    expect(screen.getByText('https://example.com/hook2')).toBeInTheDocument();
    expect(screen.getByText('agent.created')).toBeInTheDocument();
    expect(screen.getByText('agent.updated')).toBeInTheDocument();
    expect(screen.getByText('mcp.created')).toBeInTheDocument();
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    render(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByTestId('webhooks-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('opens create modal and submits webhook, then shows secret', async () => {
    mockFetchWebhooks([], 0);
    render(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('webhooks-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('button', { name: /create webhook/i }));

    expect(screen.getByRole('dialog', { name: /create webhook/i })).toBeInTheDocument();

    await ue.type(document.getElementById('webhook-url')!, 'https://example.com/new');
    await ue.click(screen.getByLabelText('agent.created'));

    // Mock POST response with secret
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: {
          webhook: { ...mockWebhook1, id: 'wh-3', url: 'https://example.com/new' },
          secret: 'whsec_test_secret_123',
        },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-3' },
      }),
    } as Response);
    // Mock refetch
    mockFetchWebhooks([mockWebhook1], 1);

    await ue.click(screen.getByTestId('create-webhook-submit'));

    await waitFor(() => {
      expect(screen.getByTestId('secret-modal')).toBeInTheDocument();
    });

    // ClipboardCopy renders value in an input element or as text
    const secretCopy = screen.getByTestId('webhook-secret-copy');
    expect(secretCopy).toBeInTheDocument();
    const secretInput = secretCopy.querySelector('input');
    if (secretInput) {
      expect(secretInput).toHaveValue('whsec_test_secret_123');
    } else {
      expect(secretCopy).toHaveTextContent('whsec_test_secret_123');
    }
  });

  it('deletes a webhook after confirmation', async () => {
    mockFetchWebhooks([mockWebhook1], 1);
    render(<WebhooksPage />);

    await waitFor(() => {
      expect(screen.getByTestId('webhooks-table')).toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('delete-webhook-wh-1'));

    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();

    // Mock DELETE
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 204,
    } as Response);
    mockFetchWebhooks([], 0);

    await ue.click(screen.getByTestId('confirm-button'));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
      expect(deleteCall![0]).toContain('/api/v1/webhooks/wh-1');
    });
  });
});
