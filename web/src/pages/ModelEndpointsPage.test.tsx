import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { ModelEndpointsPage } from './ModelEndpointsPage';
import type { ModelEndpoint } from '../types';

const mockNavigate = vi.fn();
vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom');
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  };
});

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockAddToast = vi.fn();
vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: mockAddToast }),
}));

const mockEndpoint1: ModelEndpoint = {
  id: 'ep-1',
  slug: 'openai-gpt4o-prod',
  name: 'GPT-4o Production',
  provider: 'openai',
  endpoint_url: 'https://api.openai.com/v1',
  is_fixed_model: true,
  model_name: 'gpt-4o-2024-08-06',
  allowed_models: [],
  is_active: true,
  workspace_id: null,
  version: 3,
  created_by: 'admin',
  created_at: '2026-01-15T10:00:00Z',
  updated_at: '2026-02-10T14:30:00Z',
};

const mockEndpoint2: ModelEndpoint = {
  id: 'ep-2',
  slug: 'azure-east-flex',
  name: 'Azure East Flexible',
  provider: 'azure',
  endpoint_url: 'https://myorg-east.openai.azure.com',
  is_fixed_model: false,
  model_name: 'gpt-4o',
  allowed_models: ['gpt-4o', 'gpt-4o-mini', 'gpt-4-turbo'],
  is_active: false,
  workspace_id: null,
  version: 1,
  created_by: 'admin',
  created_at: '2026-01-20T08:00:00Z',
  updated_at: '2026-01-20T08:00:00Z',
};

let fetchMock: Mock;

function mockFetchEndpoints(endpoints: ModelEndpoint[], total: number) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { endpoints, total },
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
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err' },
    }),
  } as Response);
}

function mockDeleteSuccess() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 204,
  } as Response);
}

function mockCreateSuccess() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 201,
    json: async () => ({
      success: true,
      data: mockEndpoint1,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-create' },
    }),
  } as Response);
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/model-endpoints']}>
      <ModelEndpointsPage />
    </MemoryRouter>,
  );
}

describe('ModelEndpointsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockNavigate.mockReset();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows a loading spinner initially', () => {
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();
    expect(screen.getByTestId('endpoints-loading')).toBeInTheDocument();
  });

  it('renders endpoint table with correct data after loading', async () => {
    mockFetchEndpoints([mockEndpoint1, mockEndpoint2], 2);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Model Endpoints')).toBeInTheDocument();
    expect(screen.getByText('GPT-4o Production')).toBeInTheDocument();
    expect(screen.getByText('Azure East Flexible')).toBeInTheDocument();
    expect(screen.getByText('openai-gpt4o-prod')).toBeInTheDocument();
    expect(screen.getByText('azure-east-flex')).toBeInTheDocument();
  });

  it('displays provider labels with correct text', async () => {
    mockFetchEndpoints([mockEndpoint1, mockEndpoint2], 2);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('openai')).toBeInTheDocument();
    expect(screen.getByText('azure')).toBeInTheDocument();
  });

  it('displays active/inactive status labels', async () => {
    mockFetchEndpoints([mockEndpoint1, mockEndpoint2], 2);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    const activeLabels = screen.getAllByText('Active');
    const inactiveLabels = screen.getAllByText('Inactive');
    expect(activeLabels.length).toBeGreaterThanOrEqual(1);
    expect(inactiveLabels.length).toBeGreaterThanOrEqual(1);
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId('endpoints-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('shows empty state when no endpoints exist', async () => {
    mockFetchEndpoints([], 0);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText(/no model endpoints found/i)).toBeInTheDocument();
  });

  it('navigates to endpoint detail on row click', async () => {
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    await userEvent.setup().click(screen.getByText('GPT-4o Production'));

    expect(mockNavigate).toHaveBeenCalledWith('/model-endpoints/openai-gpt4o-prod');
  });

  it('shows Create Endpoint button for admin', async () => {
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('create-endpoint-btn')).toBeInTheDocument();
  });

  it('hides Create Endpoint button for viewer', async () => {
    mockUser = { role: 'viewer' };
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('create-endpoint-btn')).not.toBeInTheDocument();
  });

  it('hides kebab actions for viewer', async () => {
    mockUser = { role: 'viewer' };
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('endpoint-actions-openai-gpt4o-prod')).not.toBeInTheDocument();
  });

  it('opens create modal and submits new endpoint', async () => {
    mockFetchEndpoints([], 0);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('create-endpoint-btn'));

    // Modal should be open
    expect(screen.getByText('Create Model Endpoint')).toBeInTheDocument();

    // Fill form fields
    await ue.type(document.getElementById('create-endpoint-slug')!, 'openai-gpt4o-prod');
    await ue.type(document.getElementById('create-endpoint-name')!, 'GPT-4o Production');
    await ue.type(document.getElementById('create-endpoint-url')!, 'https://api.openai.com/v1');
    await ue.type(document.getElementById('create-endpoint-model')!, 'gpt-4o');

    // Submit
    mockCreateSuccess();
    mockFetchEndpoints([mockEndpoint1], 1);

    await ue.click(screen.getByTestId('create-endpoint-submit'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'POST',
      );
      expect(postCall).toBeDefined();
      expect(postCall![0]).toBe('/api/v1/model-endpoints');
    });
  });

  it('filters endpoints by name using search input', async () => {
    mockFetchEndpoints([mockEndpoint1, mockEndpoint2], 2);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/filter by name or slug/i);
    await userEvent.setup().type(searchInput, 'GPT');

    expect(screen.getByText('GPT-4o Production')).toBeInTheDocument();
    expect(screen.queryByText('Azure East Flexible')).not.toBeInTheDocument();
  });

  it('filters endpoints by slug using search input', async () => {
    mockFetchEndpoints([mockEndpoint1, mockEndpoint2], 2);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    const searchInput = screen.getByPlaceholderText(/filter by name or slug/i);
    await userEvent.setup().type(searchInput, 'azure');

    expect(screen.queryByText('GPT-4o Production')).not.toBeInTheDocument();
    expect(screen.getByText('Azure East Flexible')).toBeInTheDocument();
  });

  it('calls DELETE API on delete action', async () => {
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Open kebab menu
    const kebab = screen.getByTestId('endpoint-actions-openai-gpt4o-prod').querySelector('button')!;
    await ue.click(kebab);

    // Click delete in dropdown
    const deleteItem = await screen.findByRole('menuitem', { name: /delete/i });
    await ue.click(deleteItem);

    // ConfirmDialog should appear
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();

    // Confirm deletion
    mockDeleteSuccess();
    mockFetchEndpoints([], 0);
    await ue.click(screen.getByTestId('confirm-button'));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'DELETE',
      );
      expect(deleteCall).toBeDefined();
      expect(deleteCall![0]).toContain('/api/v1/model-endpoints/openai-gpt4o-prod');
    });
  });

  it('shows model with allowed_models count for flexible endpoint', async () => {
    mockFetchEndpoints([mockEndpoint2], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    // Flexible endpoint shows model_name + count of allowed_models
    expect(screen.getByText('gpt-4o (+3)')).toBeInTheDocument();
  });

  it('fetches from correct API endpoint', async () => {
    mockFetchEndpoints([mockEndpoint1], 1);
    renderPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoints-loading')).not.toBeInTheDocument();
    });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/model-endpoints');
  });
});
