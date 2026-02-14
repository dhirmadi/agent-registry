import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { ModelConfigPage } from './ModelConfigPage';
import type { ModelConfig } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockConfig: ModelConfig = {
  id: 'mc-1',
  scope: 'global',
  scope_id: '',
  default_model: 'claude-sonnet-4-5-20250929',
  temperature: 0.7,
  max_tokens: 4096,
  max_tool_rounds: 10,
  default_context_window: 128000,
  default_max_output_tokens: 8192,
  history_token_budget: 32000,
  max_history_messages: 50,
  embedding_model: 'text-embedding-3-small',
  updated_at: '2026-01-01T00:00:00Z',
};

let fetchMock: Mock;

function mockFetchConfigSuccess(config: ModelConfig = mockConfig) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: config,
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchConfigError() {
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

describe('ModelConfigPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows loading spinner initially', () => {
    mockFetchConfigSuccess();
    render(<ModelConfigPage />);
    expect(screen.getByTestId('model-config-loading')).toBeInTheDocument();
  });

  it('renders config form after loading', async () => {
    mockFetchConfigSuccess();
    render(<ModelConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('model-config-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Model Configuration')).toBeInTheDocument();
    expect(screen.getByTestId('model-config-form')).toBeInTheDocument();

    const modelInput = document.getElementById('default-model') as HTMLInputElement;
    expect(modelInput.value).toBe('claude-sonnet-4-5-20250929');

    const maxTokensInput = document.getElementById('max-tokens') as HTMLInputElement;
    expect(maxTokensInput.value).toBe('4096');

    const embeddingInput = document.getElementById('embedding-model') as HTMLInputElement;
    expect(embeddingInput.value).toBe('text-embedding-3-small');
  });

  it('shows error when API fails', async () => {
    mockFetchConfigError();
    render(<ModelConfigPage />);

    await waitFor(() => {
      expect(screen.getByTestId('model-config-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('saves config via PUT with If-Match', async () => {
    mockFetchConfigSuccess();
    render(<ModelConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('model-config-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Modify a field
    const modelInput = document.getElementById('default-model') as HTMLInputElement;
    await ue.clear(modelInput);
    await ue.type(modelInput, 'gpt-4o');

    // Mock PUT success and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockConfig, default_model: 'gpt-4o' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-put' },
      }),
    } as Response);
    mockFetchConfigSuccess({ ...mockConfig, default_model: 'gpt-4o' });

    await ue.click(screen.getByTestId('save-model-config-btn'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      expect((putCall![0] as string)).toContain('/api/v1/config/model');
      const headers = new Headers((putCall![1] as RequestInit).headers);
      expect(headers.get('If-Match')).toBe('2026-01-01T00:00:00Z');
    });
  });

  it('hides save button for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockFetchConfigSuccess();
    render(<ModelConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('model-config-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('save-model-config-btn')).not.toBeInTheDocument();
  });

  it('disables form inputs for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockFetchConfigSuccess();
    render(<ModelConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('model-config-loading')).not.toBeInTheDocument();
    });

    const modelInput = document.getElementById('default-model') as HTMLInputElement;
    expect(modelInput.disabled).toBe(true);
  });
});
