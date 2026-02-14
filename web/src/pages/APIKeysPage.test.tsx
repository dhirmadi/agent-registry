import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { APIKeysPage } from './APIKeysPage';
import type { APIKey } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockKey1: APIKey = {
  id: 'key-1',
  name: 'CI Pipeline',
  key_prefix: 'ar_ci_',
  scopes: ['read', 'write'],
  is_active: true,
  created_at: '2026-01-15T10:00:00Z',
  expires_at: '2027-01-15T10:00:00Z',
  last_used_at: '2026-02-10T14:30:00Z',
};

const mockKey2: APIKey = {
  id: 'key-2',
  name: 'Read-Only Key',
  key_prefix: 'ar_ro_',
  scopes: ['read'],
  is_active: false,
  created_at: '2026-01-20T08:00:00Z',
  expires_at: null,
  last_used_at: null,
};

let fetchMock: Mock;

function mockFetchKeys(keys: APIKey[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: keys, total },
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

describe('APIKeysPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows a loading spinner initially', () => {
    mockFetchKeys([mockKey1], 1);
    render(<APIKeysPage />);
    expect(screen.getByTestId('apikeys-loading')).toBeInTheDocument();
  });

  it('renders API key table with correct data after loading', async () => {
    mockFetchKeys([mockKey1, mockKey2], 2);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('apikeys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('API Keys')).toBeInTheDocument();
    expect(screen.getByTestId('apikeys-table')).toBeInTheDocument();
    expect(screen.getByText('CI Pipeline')).toBeInTheDocument();
    expect(screen.getByText('Read-Only Key')).toBeInTheDocument();
    expect(screen.getByText('ar_ci_')).toBeInTheDocument();
    expect(screen.getByText('ar_ro_')).toBeInTheDocument();
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByTestId('apikeys-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('opens create modal and submits key, then shows raw key', async () => {
    mockFetchKeys([], 0);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('apikeys-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('button', { name: /create api key/i }));

    expect(screen.getByRole('dialog', { name: /create api key/i })).toBeInTheDocument();

    await ue.type(document.getElementById('apikey-name')!, 'Test Key');
    await ue.click(screen.getByLabelText('read'));

    // Mock POST response with raw_key
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: {
          key: { ...mockKey1, id: 'key-3', name: 'Test Key' },
          raw_key: 'ar_test_abcdef123456',
        },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-3' },
      }),
    } as Response);
    // Mock refetch
    mockFetchKeys([mockKey1], 1);

    await ue.click(screen.getByTestId('create-apikey-submit'));

    await waitFor(() => {
      expect(screen.getByTestId('rawkey-modal')).toBeInTheDocument();
    });

    // ClipboardCopy renders value in an input element or as text
    const rawKeyCopy = screen.getByTestId('raw-key-copy');
    expect(rawKeyCopy).toBeInTheDocument();
    const rawKeyInput = rawKeyCopy.querySelector('input');
    if (rawKeyInput) {
      expect(rawKeyInput).toHaveValue('ar_test_abcdef123456');
    } else {
      expect(rawKeyCopy).toHaveTextContent('ar_test_abcdef123456');
    }
  });

  it('revokes an API key after confirmation', async () => {
    mockFetchKeys([mockKey1], 1);
    render(<APIKeysPage />);

    await waitFor(() => {
      expect(screen.getByTestId('apikeys-table')).toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('revoke-key-key-1'));

    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();

    // Mock DELETE
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 204,
    } as Response);
    mockFetchKeys([], 0);

    await ue.click(screen.getByTestId('confirm-button'));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
      expect(deleteCall![0]).toContain('/api/v1/api-keys/key-1');
    });
  });
});
