import { render, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { BrowserRouter } from 'react-router-dom';
import { AuthProvider, useAuth } from './AuthContext';
import type { User, LoginResponse } from '../types';

const mockUser: User = {
  id: '1',
  username: 'admin',
  email: 'admin@example.com',
  display_name: 'Admin User',
  role: 'admin',
  auth_method: 'password',
  is_active: true,
  last_login_at: null,
};

// Helper component to expose auth context for testing
function AuthConsumer() {
  const { user, loading } = useAuth();
  if (loading) return <div>Loading...</div>;
  if (user) return <div>Logged in as {user.username}</div>;
  return <div>Not logged in</div>;
}

function renderWithProviders(ui: React.ReactElement) {
  return render(
    <BrowserRouter>
      {ui}
    </BrowserRouter>,
  );
}

describe('AuthContext', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('shows loading state initially then renders user after /auth/me succeeds', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: mockUser,
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: '123' },
      }),
    } as Response);

    renderWithProviders(
      <AuthProvider>
        <AuthConsumer />
      </AuthProvider>,
    );

    expect(screen.getByText('Loading...')).toBeInTheDocument();

    await waitFor(() => {
      expect(screen.getByText('Logged in as admin')).toBeInTheDocument();
    });
  });

  it('shows not logged in when /auth/me fails', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: false,
      status: 401,
      json: async () => ({
        success: false,
        data: null,
        error: { code: 'UNAUTHORIZED', message: 'not authenticated' },
        meta: { timestamp: new Date().toISOString(), request_id: '123' },
      }),
    } as Response);

    renderWithProviders(
      <AuthProvider>
        <AuthConsumer />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Not logged in')).toBeInTheDocument();
    });
  });

  it('login sets user on success', async () => {
    const loginResponse: LoginResponse = {
      user: mockUser,
      must_change_password: false,
    };

    // First call: /auth/me fails (not logged in)
    // Second call: /auth/login succeeds
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce({
        ok: false,
        status: 401,
        json: async () => ({
          success: false,
          data: null,
          error: { code: 'UNAUTHORIZED', message: 'not authenticated' },
          meta: { timestamp: new Date().toISOString(), request_id: '123' },
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          data: loginResponse,
          error: null,
          meta: { timestamp: new Date().toISOString(), request_id: '456' },
        }),
      } as Response);

    function LoginTrigger() {
      const { user, loading, login } = useAuth();
      if (loading) return <div>Loading...</div>;
      if (user) return <div>Logged in as {user.username}</div>;
      return <button onClick={() => login('admin', 'password123')}>Log In</button>;
    }

    renderWithProviders(
      <AuthProvider>
        <LoginTrigger />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Log In')).toBeInTheDocument();
    });

    const { default: userEvent } = await import('@testing-library/user-event');
    const ue = userEvent.setup();
    await ue.click(screen.getByText('Log In'));

    await waitFor(() => {
      expect(screen.getByText('Logged in as admin')).toBeInTheDocument();
    });
  });

  it('logout clears user state', async () => {
    // First call: /auth/me succeeds
    // Second call: /auth/logout succeeds
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          data: mockUser,
          error: null,
          meta: { timestamp: new Date().toISOString(), request_id: '123' },
        }),
      } as Response)
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          success: true,
          data: { message: 'logged out' },
          error: null,
          meta: { timestamp: new Date().toISOString(), request_id: '456' },
        }),
      } as Response);

    function LogoutTrigger() {
      const { user, loading, logout } = useAuth();
      if (loading) return <div>Loading...</div>;
      if (user) return <button onClick={logout}>Log Out</button>;
      return <div>Not logged in</div>;
    }

    renderWithProviders(
      <AuthProvider>
        <LogoutTrigger />
      </AuthProvider>,
    );

    await waitFor(() => {
      expect(screen.getByText('Log Out')).toBeInTheDocument();
    });

    const { default: userEvent } = await import('@testing-library/user-event');
    const ue = userEvent.setup();
    await ue.click(screen.getByText('Log Out'));

    await waitFor(() => {
      expect(screen.getByText('Not logged in')).toBeInTheDocument();
    });
  });

  it('throws error when useAuth is used outside AuthProvider', () => {
    // Suppress console.error for this test since React will log the error
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

    function Orphan() {
      useAuth();
      return null;
    }

    expect(() => render(<Orphan />)).toThrow('useAuth must be used within an AuthProvider');

    consoleSpy.mockRestore();
  });
});
