import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { ModelEndpointDetailPage } from './ModelEndpointDetailPage';
import type { ModelEndpoint, ModelEndpointVersion } from '../types';

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

const mockEndpoint: ModelEndpoint = {
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

const mockVersions: ModelEndpointVersion[] = [
  {
    id: 'ver-3',
    endpoint_id: 'ep-1',
    version: 3,
    config: { temperature: 0.3, max_tokens: 4096, context_window: 128000 },
    is_active: true,
    change_note: 'Lowered temperature for deterministic output',
    created_by: 'admin',
    created_at: '2026-02-10T14:30:00Z',
  },
  {
    id: 'ver-2',
    endpoint_id: 'ep-1',
    version: 2,
    config: { temperature: 0.7, max_tokens: 4096 },
    is_active: false,
    change_note: 'Initial production config',
    created_by: 'admin',
    created_at: '2026-02-08T10:00:00Z',
  },
];

let fetchMock: Mock;

function mockFetchEndpoint() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: mockEndpoint,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchVersions() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { versions: mockVersions, total: 2 },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-2' },
    }),
  } as Response);
}

function mockFetchError() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 404,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'NOT_FOUND', message: "endpoint 'nonexistent' not found" },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-err' },
    }),
  } as Response);
}

function mockSaveSuccess() {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { ...mockEndpoint, updated_at: '2026-02-14T12:00:00Z' },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-save' },
    }),
  } as Response);
}

function mockSaveConflict() {
  fetchMock.mockResolvedValueOnce({
    ok: false,
    status: 409,
    json: async () => ({
      success: false,
      data: null,
      error: { code: 'CONFLICT', message: 'resource was modified by another client' },
      meta: { timestamp: new Date().toISOString(), request_id: 'req-conflict' },
    }),
  } as Response);
}

function mockAllFetches() {
  mockFetchEndpoint();
  mockFetchVersions();
}

function renderDetailPage(slug = 'openai-gpt4o-prod') {
  return render(
    <MemoryRouter initialEntries={[`/model-endpoints/${slug}`]}>
      <Routes>
        <Route path="/model-endpoints/:slug" element={<ModelEndpointDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('ModelEndpointDetailPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    mockNavigate.mockReset();
    mockAddToast.mockReset();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows a loading spinner initially', () => {
    mockAllFetches();
    renderDetailPage();
    expect(screen.getByTestId('endpoint-detail-loading')).toBeInTheDocument();
  });

  it('renders endpoint name and details after loading', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('GPT-4o Production')).toBeInTheDocument();
    expect(screen.getByText('openai-gpt4o-prod')).toBeInTheDocument();
    // 'openai' appears in the provider label and FormSelect option
    expect(screen.getAllByText('openai').length).toBeGreaterThanOrEqual(1);
  });

  it('shows error when endpoint is not found', async () => {
    mockFetchError();
    fetchMock.mockResolvedValueOnce({
      ok: false,
      status: 404,
      json: async () => ({
        success: false,
        data: null,
        error: { code: 'NOT_FOUND', message: 'not found' },
        meta: { timestamp: new Date().toISOString(), request_id: 'req-4' },
      }),
    } as Response);
    renderDetailPage('nonexistent');

    await waitFor(() => {
      expect(screen.getByTestId('endpoint-detail-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/not found/i)).toBeInTheDocument();
  });

  it('displays Overview tab with editable form fields', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByLabelText(/^name$/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/endpoint url/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/model name/i)).toBeInTheDocument();
  });

  it('saves Overview tab via PUT with If-Match', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    const nameInput = screen.getByLabelText(/^name$/i);
    await ue.clear(nameInput);
    await ue.type(nameInput, 'Updated GPT-4o');

    mockSaveSuccess();
    mockAllFetches();
    await ue.click(screen.getByTestId('save-overview'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      expect(putCall![0]).toBe('/api/v1/model-endpoints/openai-gpt4o-prod');
      const headers = new Headers((putCall![1] as RequestInit).headers);
      expect(headers.get('If-Match')).toBe(mockEndpoint.updated_at);
    });
  });

  it('shows conflict warning on 409 response', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    const nameInput = screen.getByLabelText(/^name$/i);
    await ue.clear(nameInput);
    await ue.type(nameInput, 'Conflicting name');

    mockSaveConflict();
    await ue.click(screen.getByTestId('save-overview'));

    await waitFor(() => {
      expect(screen.getByText(/modified by another/i)).toBeInTheDocument();
    });
  });

  it('switches to Configuration tab and shows active version config', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /configuration/i }));

    expect(screen.getByText(/active version: v3/i)).toBeInTheDocument();
    // Change note text may appear in both config tab and version table
    expect(screen.getAllByText(/lowered temperature/i).length).toBeGreaterThanOrEqual(1);
  });

  it('shows Create New Version button on Configuration tab', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /configuration/i }));

    expect(screen.getByTestId('create-version-btn')).toBeInTheDocument();
  });

  it('opens Create Version modal and submits', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /configuration/i }));
    await ue.click(screen.getByTestId('create-version-btn'));

    // Modal should be open -- text appears in button and modal title
    expect(screen.getAllByText('Create New Version').length).toBeGreaterThanOrEqual(1);

    // Fill change note
    await ue.type(document.getElementById('version-change-note')!, 'New config');

    // Submit
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 201,
      json: async () => ({
        success: true,
        data: { id: 'ver-4', endpoint_id: 'ep-1', version: 4 },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-create-ver' },
      }),
    } as Response);
    mockAllFetches();

    await ue.click(screen.getByTestId('create-version-submit'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => {
          const url = call[0] as string;
          return url.includes('/versions') && (call[1] as RequestInit)?.method === 'POST';
        },
      );
      expect(postCall).toBeDefined();
      expect(postCall![0]).toBe('/api/v1/model-endpoints/openai-gpt4o-prod/versions');
    });
  });

  it('switches to Version History tab and shows versions', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /version history/i }));

    expect(screen.getAllByText(/v3/).length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/v2/).length).toBeGreaterThanOrEqual(1);
    // v2 (non-active) should have rollback/activate button
    expect(screen.getByTestId('rollback-2')).toBeInTheDocument();
  });

  it('activates a version when Activate button is clicked', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /version history/i }));

    // Mock activate response
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { message: 'activated' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-activate' },
      }),
    } as Response);
    mockAllFetches();

    await ue.click(screen.getByTestId('activate-version-2'));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (call: unknown[]) => {
          const url = call[0] as string;
          return url.includes('/activate') && (call[1] as RequestInit)?.method === 'POST';
        },
      );
      expect(postCall).toBeDefined();
      expect(postCall![0]).toBe('/api/v1/model-endpoints/openai-gpt4o-prod/versions/2/activate');
    });
  });

  it('shows active badge and version number in header', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.getAllByText('Active').length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText(/v3/).length).toBeGreaterThanOrEqual(1);
  });

  it('provides Back to Model Endpoints navigation', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    const backButton = screen.getByRole('button', { name: /back to model endpoints/i });
    await ue.click(backButton);

    expect(mockNavigate).toHaveBeenCalledWith('/model-endpoints');
  });

  it('fetches endpoint data from correct URL', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/model-endpoints/openai-gpt4o-prod');
  });

  it('fetches versions from correct URL', async () => {
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    expect(fetchMock.mock.calls[1][0]).toBe('/api/v1/model-endpoints/openai-gpt4o-prod/versions');
  });

  it('hides save buttons for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('save-overview')).not.toBeInTheDocument();
  });

  it('hides Create New Version button for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockAllFetches();
    renderDetailPage();

    await waitFor(() => {
      expect(screen.queryByTestId('endpoint-detail-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.click(screen.getByRole('tab', { name: /configuration/i }));

    expect(screen.queryByTestId('create-version-btn')).not.toBeInTheDocument();
  });
});
