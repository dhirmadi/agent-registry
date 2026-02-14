import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { UsersPage } from './UsersPage';
import type { UserAdmin } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockUser1: UserAdmin = {
  id: 'usr-1',
  username: 'admin',
  email: 'admin@example.com',
  display_name: 'Admin User',
  role: 'admin',
  auth_method: 'password',
  is_active: true,
  must_change_pass: false,
  last_login_at: '2026-02-10T14:30:00Z',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const mockUser2: UserAdmin = {
  id: 'usr-2',
  username: 'editor1',
  email: 'editor@example.com',
  display_name: 'Editor User',
  role: 'editor',
  auth_method: 'google',
  is_active: true,
  must_change_pass: false,
  last_login_at: null,
  created_at: '2026-01-15T00:00:00Z',
  updated_at: '2026-01-15T00:00:00Z',
};

let fetchMock: Mock;

function mockFetchUsers(users: UserAdmin[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: users, total },
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

describe('UsersPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows access denied for non-admin users', () => {
    mockUser = { role: 'viewer' };
    render(<UsersPage />);
    expect(screen.getByTestId('users-access-denied')).toBeInTheDocument();
  });

  it('shows a loading spinner initially', () => {
    mockFetchUsers([mockUser1], 1);
    render(<UsersPage />);
    expect(screen.getByTestId('users-loading')).toBeInTheDocument();
  });

  it('renders user table with correct data after loading', async () => {
    mockFetchUsers([mockUser1, mockUser2], 2);
    render(<UsersPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('users-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Users')).toBeInTheDocument();
    expect(screen.getByTestId('users-table')).toBeInTheDocument();
    expect(screen.getByText('admin@example.com')).toBeInTheDocument();
    expect(screen.getByText('editor@example.com')).toBeInTheDocument();
    expect(screen.getByText('Admin User')).toBeInTheDocument();
    expect(screen.getByText('Editor User')).toBeInTheDocument();
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    render(<UsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('users-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('opens create modal and submits new user', async () => {
    mockFetchUsers([], 0);
    render(<UsersPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('users-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('button', { name: /create user/i }));

    expect(screen.getByRole('dialog', { name: /create user/i })).toBeInTheDocument();

    await ue.type(document.getElementById('user-username')!, 'newuser');
    await ue.type(document.getElementById('user-email')!, 'new@example.com');
    await ue.type(document.getElementById('user-display-name')!, 'New User');
    await ue.type(document.getElementById('user-password')!, 'TempPass123!');

    // Mock POST
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: { ...mockUser1, id: 'usr-3', username: 'newuser' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-3' },
      }),
    } as Response);
    mockFetchUsers([mockUser1], 1);

    await ue.click(screen.getByTestId('create-user-submit'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
      expect(postCall![0]).toBe('/api/v1/users');
    });
  });

  it('opens edit modal and updates user role', async () => {
    mockFetchUsers([mockUser2], 1);
    render(<UsersPage />);

    await waitFor(() => {
      expect(screen.getByTestId('users-table')).toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('edit-user-usr-2'));

    await waitFor(() => {
      expect(screen.getByTestId('edit-user-modal')).toBeInTheDocument();
    });

    // Mock PATCH
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockUser2, role: 'viewer' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-4' },
      }),
    } as Response);
    mockFetchUsers([{ ...mockUser2, role: 'viewer' }], 1);

    await ue.click(screen.getByTestId('edit-user-submit'));

    await waitFor(() => {
      const patchCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PATCH',
      );
      expect(patchCall).toBeDefined();
    });
  });
});
