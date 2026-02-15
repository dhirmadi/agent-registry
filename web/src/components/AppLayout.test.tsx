import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { AppLayout } from './AppLayout';
import type { User } from '../types';

const mockLogout = vi.fn();
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

let currentMockUser: User | null = mockUser;

vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({
    user: currentMockUser,
    loading: false,
    login: vi.fn(),
    logout: mockLogout,
  }),
}));

function renderLayout(initialRoute = '/') {
  return render(
    <MemoryRouter initialEntries={[initialRoute]}>
      <Routes>
        <Route path="/" element={<AppLayout />}>
          <Route index element={<div>Dashboard Content</div>} />
        </Route>
      </Routes>
    </MemoryRouter>,
  );
}

describe('AppLayout', () => {
  beforeEach(() => {
    currentMockUser = mockUser;
    vi.clearAllMocks();
  });

  it('renders the app title', () => {
    renderLayout();
    expect(screen.getByText('Agentic Registry')).toBeInTheDocument();
  });

  it('renders the user display name and role', () => {
    renderLayout();
    expect(screen.getByText(/Admin User/)).toBeInTheDocument();
    expect(screen.getByText('admin')).toBeInTheDocument();
  });

  it('renders primary navigation items', () => {
    renderLayout();
    expect(screen.getByText('Dashboard')).toBeInTheDocument();
    expect(screen.getByText('Agents')).toBeInTheDocument();
    expect(screen.getByText('Prompts')).toBeInTheDocument();
    expect(screen.getByText('MCP Servers')).toBeInTheDocument();
    expect(screen.getByText('Trust Rules')).toBeInTheDocument();
    expect(screen.getByText('Model Config')).toBeInTheDocument();
    expect(screen.getByText('Webhooks')).toBeInTheDocument();
    expect(screen.getByText('API Keys')).toBeInTheDocument();
    expect(screen.getByText('Audit Log')).toBeInTheDocument();
    expect(screen.getByText('My Account')).toBeInTheDocument();
  });

  it('shows Users nav item for admin role', () => {
    renderLayout();
    expect(screen.getByText('Users')).toBeInTheDocument();
  });

  it('hides Users nav item for non-admin role', () => {
    currentMockUser = { ...mockUser, role: 'editor' };
    renderLayout();
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
  });

  it('renders the Outlet content', () => {
    renderLayout();
    expect(screen.getByText('Dashboard Content')).toBeInTheDocument();
  });

  it('calls logout when logout button is clicked', async () => {
    const user = userEvent.setup();
    renderLayout();

    await user.click(screen.getByText('Logout'));
    expect(mockLogout).toHaveBeenCalledTimes(1);
  });
});
