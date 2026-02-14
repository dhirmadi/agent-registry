import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter, Routes, Route, Navigate } from 'react-router-dom';
import { ProtectedRoute } from '../auth/ProtectedRoute';
import { ToastProvider } from '../components/ToastNotifications';
import { LoginPage } from '../auth/LoginPage';
import { AppLayout } from '../components/AppLayout';
import { DashboardPage } from '../pages/DashboardPage';
import { AgentsPage } from '../pages/AgentsPage';
import { AuditLogPage } from '../pages/AuditLogPage';
import { MyAccountPage } from '../pages/MyAccountPage';
import type { User } from '../types';

// --- Mock auth context ---
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

let authState: {
  user: User | null;
  loading: boolean;
  mustChangePassword: boolean;
  login: Mock;
  logout: Mock;
};

vi.mock('../auth/AuthContext', () => ({
  useAuth: () => authState,
  AuthProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

// --- Helpers ---

/** Build the route tree that mirrors App.tsx, but using MemoryRouter for testability. */
function renderApp(initialRoute = '/') {
  return render(
    <MemoryRouter initialEntries={[initialRoute]}>
      <ToastProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            path="/"
            element={
              <ProtectedRoute>
                <AppLayout />
              </ProtectedRoute>
            }
          >
            <Route index element={<DashboardPage />} />
            <Route path="agents" element={<AgentsPage />} />
            <Route path="audit-log" element={<AuditLogPage />} />
            <Route path="my-account" element={<MyAccountPage />} />
          </Route>
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </ToastProvider>
    </MemoryRouter>,
  );
}

/** Create a mock fetch response in the standard API envelope format. */
function mockEnvelopeResponse<T>(data: T, status = 200): Partial<Response> {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => ({
      success: status >= 200 && status < 300,
      data,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-smoke' },
    }),
  };
}

/** Mock discovery response for the dashboard page. */
function mockDiscoveryResponse() {
  return mockEnvelopeResponse({
    agents: [],
    mcp_servers: [],
    trust_defaults: [],
    model_config: {},
    context_config: {},
    signal_config: [],
    fetched_at: new Date().toISOString(),
  });
}

/** Mock agents list response. */
function mockAgentsListResponse() {
  return mockEnvelopeResponse({ items: [], total: 0 });
}

/** Mock audit log response. */
function mockAuditLogResponse() {
  return mockEnvelopeResponse({ items: [], total: 0, offset: 0, limit: 50 });
}

/** Mock API keys list response for MyAccountPage. */
function mockApiKeysListResponse() {
  return mockEnvelopeResponse({ items: [], total: 0 });
}

let fetchMock: Mock;

describe('App smoke tests', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;

    // Default: authenticated admin user
    authState = {
      user: mockUser,
      loading: false,
      mustChangePassword: false,
      login: vi.fn(),
      logout: vi.fn(),
    };
  });

  it('authenticated user sees the dashboard', async () => {
    fetchMock.mockResolvedValueOnce(mockDiscoveryResponse());
    renderApp('/');

    // AppLayout renders the title
    expect(screen.getByText('Agentic Registry')).toBeInTheDocument();

    // Dashboard heading appears after loading
    await waitFor(() => {
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });
  });

  it('navigates to agents page via sidebar link', async () => {
    // First render loads dashboard (discovery call)
    fetchMock.mockResolvedValueOnce(mockDiscoveryResponse());
    renderApp('/');

    await waitFor(() => {
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    // Mock the agents API call that will fire when AgentsPage mounts
    fetchMock.mockResolvedValueOnce(mockAgentsListResponse());

    const ue = userEvent.setup();
    // Click the "Agents" nav link in the sidebar
    const agentsLink = screen.getByRole('link', { name: /^Agents$/i });
    await ue.click(agentsLink);

    // AgentsPage should now be rendered -- look for the h1 specifically
    await waitFor(() => {
      expect(screen.getByRole('heading', { level: 1, name: /agents/i })).toBeInTheDocument();
    });
  });

  it('navigates to audit log page via sidebar link', async () => {
    fetchMock.mockResolvedValueOnce(mockDiscoveryResponse());
    renderApp('/');

    await waitFor(() => {
      expect(screen.getByText('Dashboard')).toBeInTheDocument();
    });

    fetchMock.mockResolvedValueOnce(mockAuditLogResponse());

    const ue = userEvent.setup();
    const auditLogLink = screen.getByRole('link', { name: /^Audit Log$/i });
    await ue.click(auditLogLink);

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /audit log/i })).toBeInTheDocument();
    });
  });

  it('renders my account page at /my-account', async () => {
    // MyAccountPage fetches API keys on mount
    fetchMock.mockResolvedValueOnce(mockApiKeysListResponse());
    renderApp('/my-account');

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /my account/i })).toBeInTheDocument();
    });

    // Account info card should display the mocked user data.
    // "Admin User" also appears in the header, so check that at least one instance is present.
    const adminUserTexts = screen.getAllByText('Admin User');
    expect(adminUserTexts.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('admin@example.com')).toBeInTheDocument();
  });

  it('unauthenticated user is redirected to login', async () => {
    authState = {
      user: null,
      loading: false,
      mustChangePassword: false,
      login: vi.fn(),
      logout: vi.fn(),
    };

    renderApp('/');

    // ProtectedRoute redirects to /login when there is no user
    await waitFor(() => {
      expect(screen.getByText(/sign in to agentic registry/i)).toBeInTheDocument();
    });
  });
});
