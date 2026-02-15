import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { MyAccountPage } from './MyAccountPage';
import type { APIKey, User } from '../types';

let mockUser: Partial<User> = {
  role: 'admin',
  auth_method: 'password',
  display_name: 'Admin User',
  email: 'admin@test.com',
};

vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockAddToast = vi.fn();
vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: mockAddToast }),
}));

let fetchMock: Mock;

const mockApiKey: APIKey = {
  id: 'key-1',
  name: 'My Test Key',
  key_prefix: 'ar_test',
  scopes: ['read'],
  is_active: true,
  created_at: '2026-01-15T10:00:00Z',
  expires_at: null,
  last_used_at: null,
};

function mockFetchApiKeys(keys: APIKey[]) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { keys, total: keys.length },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function renderPage() {
  return render(
    <MemoryRouter>
      <MyAccountPage />
    </MemoryRouter>,
  );
}

describe('MyAccountPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockAddToast.mockReset();
    mockUser = {
      role: 'admin',
      auth_method: 'password',
      display_name: 'Admin User',
      email: 'admin@test.com',
    };
  });

  it('renders password change section for password users', async () => {
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('password-section')).toBeInTheDocument();
    expect(document.getElementById('current-password')).toBeInTheDocument();
    expect(document.getElementById('new-password')).toBeInTheDocument();
    expect(document.getElementById('confirm-password')).toBeInTheDocument();
  });

  it('hides password change section for google-only users', async () => {
    mockUser = {
      role: 'admin',
      auth_method: 'google',
      display_name: 'Google User',
      email: 'google@test.com',
    };
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('password-section')).not.toBeInTheDocument();
  });

  it('shows unlink button when Google is linked', async () => {
    mockUser = {
      role: 'admin',
      auth_method: 'both',
      display_name: 'Both User',
      email: 'both@test.com',
    };
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('google-section')).toBeInTheDocument();
    expect(screen.getByText('Your account is linked to Google.')).toBeInTheDocument();
    expect(screen.getByTestId('unlink-google')).toBeInTheDocument();
    expect(screen.queryByTestId('link-google')).not.toBeInTheDocument();
  });

  it('shows link button when Google is not linked', async () => {
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('google-section')).toBeInTheDocument();
    expect(screen.getByText('Your account is not linked to Google.')).toBeInTheDocument();
    expect(screen.getByTestId('link-google')).toBeInTheDocument();
    expect(screen.queryByTestId('unlink-google')).not.toBeInTheDocument();
  });

  it('renders API keys table with keys', async () => {
    mockFetchApiKeys([mockApiKey]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('api-keys-table')).toBeInTheDocument();
    expect(screen.getByText('My Test Key')).toBeInTheDocument();
    expect(screen.getByText('ar_test')).toBeInTheDocument();
    expect(screen.getByText('read')).toBeInTheDocument();
  });

  it('shows empty state when no API keys exist', async () => {
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('api-keys-table')).not.toBeInTheDocument();
    expect(screen.getByText('No API keys')).toBeInTheDocument();
  });

  it('shows password section for users with both auth methods', async () => {
    mockUser = {
      role: 'admin',
      auth_method: 'both',
      display_name: 'Both User',
      email: 'both@test.com',
    };
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('password-section')).toBeInTheDocument();
  });

  it('displays account information', async () => {
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('My Account')).toBeInTheDocument();
    expect(screen.getByText('Admin User')).toBeInTheDocument();
    expect(screen.getByText('admin@test.com')).toBeInTheDocument();
  });

  it('opens create API key modal and submits', async () => {
    mockFetchApiKeys([]);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('api-keys-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('create-api-key'));

    // Modal should be open — check for the submit button inside the modal
    expect(screen.getByTestId('submit-create-key')).toBeInTheDocument();

    // Fill name
    await ue.type(screen.getByTestId('key-name-input'), 'New Key');

    // Submit
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: { key: 'ar_secret_abc123', id: 'key-new', name: 'New Key', scopes: ['read'], key_prefix: 'ar_secr', created_at: new Date().toISOString() },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-2' },
      }),
    } as Response);
    // Re-fetch after creation
    mockFetchApiKeys([mockApiKey]);

    await ue.click(screen.getByTestId('submit-create-key'));

    await waitFor(() => {
      expect(screen.getByTestId('raw-key-display')).toBeInTheDocument();
    });
  });

  it('revokes an API key', async () => {
    mockFetchApiKeys([mockApiKey]);
    renderPage();

    await waitFor(() => {
      expect(screen.getByText('My Test Key')).toBeInTheDocument();
    });

    // Mock the DELETE call
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 204,
    } as Response);
    // Re-fetch after revocation
    mockFetchApiKeys([]);

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('revoke-key-key-1'));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
      expect(deleteCall![0]).toContain('/api/v1/api-keys/key-1');
    });
  });
});
