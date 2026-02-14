import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { LoginPage } from './LoginPage';
import { AuthProvider } from './AuthContext';
import type { LoginResponse } from '../types';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

function renderLoginPage() {
  return render(
    <MemoryRouter initialEntries={['/login']}>
      <AuthProvider>
        <LoginPage />
      </AuthProvider>
    </MemoryRouter>,
  );
}

function getUsernameInput() {
  return screen.getByRole('textbox', { name: /username/i });
}

function getPasswordInput() {
  // PatternFly LoginForm renders the password input with id "pf-login-password-id"
  return document.getElementById('pf-login-password-id') as HTMLInputElement;
}

let fetchMock: Mock;

function mockMeUnauthorized() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 401,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'UNAUTHORIZED', message: 'not authenticated' },
      meta: { timestamp: new Date().toISOString(), request_id: '123' },
    }),
  } as Response);
}

describe('LoginPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockNavigate.mockReset();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    // Default: /auth/me returns 401 (not logged in)
    mockMeUnauthorized();
  });

  it('renders login form with username and password fields', async () => {
    renderLoginPage();

    await waitFor(() => {
      expect(getUsernameInput()).toBeInTheDocument();
    });
    expect(getPasswordInput()).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument();
  });

  it('submits credentials and navigates to / on success', async () => {
    const loginResponse: LoginResponse = {
      user: {
        id: '1',
        username: 'admin',
        email: 'admin@example.com',
        display_name: 'Admin User',
        role: 'admin',
        auth_method: 'password',
        is_active: true,
        last_login_at: null,
      },
      must_change_password: false,
    };

    // Second fetch call: POST /auth/login succeeds
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: loginResponse,
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: '456' },
      }),
    } as Response);

    renderLoginPage();

    const ue = userEvent.setup();

    await waitFor(() => {
      expect(getUsernameInput()).toBeInTheDocument();
    });

    await ue.type(getUsernameInput(), 'admin');
    await ue.type(getPasswordInput(), 'password123');
    await ue.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/', { replace: true });
    });
  });

  it('navigates to /change-password when must_change_password is true', async () => {
    const loginResponse: LoginResponse = {
      user: {
        id: '1',
        username: 'admin',
        email: 'admin@example.com',
        display_name: 'Admin User',
        role: 'admin',
        auth_method: 'password',
        is_active: true,
        last_login_at: null,
      },
      must_change_password: true,
    };

    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: loginResponse,
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: '456' },
      }),
    } as Response);

    renderLoginPage();

    const ue = userEvent.setup();

    await waitFor(() => {
      expect(getUsernameInput()).toBeInTheDocument();
    });

    await ue.type(getUsernameInput(), 'admin');
    await ue.type(getPasswordInput(), 'password123');
    await ue.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(mockNavigate).toHaveBeenCalledWith('/change-password', { replace: true });
    });
  });

  it('shows error alert on login failure', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 401,
      json: async () => ({
        success: false,
        data: null,
        error: { code: 'INVALID_CREDENTIALS', message: 'Invalid username or password' },
        meta: { timestamp: new Date().toISOString(), request_id: '456' },
      }),
    } as Response);

    renderLoginPage();

    const ue = userEvent.setup();

    await waitFor(() => {
      expect(getUsernameInput()).toBeInTheDocument();
    });

    await ue.type(getUsernameInput(), 'admin');
    await ue.type(getPasswordInput(), 'wrongpassword');
    await ue.click(screen.getByRole('button', { name: /sign in/i }));

    await waitFor(() => {
      expect(screen.getByText(/invalid username or password/i)).toBeInTheDocument();
    });
  });
});
