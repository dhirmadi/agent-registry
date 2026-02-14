import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { ContextConfigPage } from './ContextConfigPage';
import type { ContextConfig } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockConfig: ContextConfig = {
  id: 'cc-1',
  scope: 'global',
  scope_id: '',
  max_total_tokens: 128000,
  layer_budgets: {
    system: 4000,
    history: 32000,
    tools: 8000,
  },
  enabled_layers: ['system', 'history', 'tools', 'rag'],
  updated_at: '2026-01-01T00:00:00Z',
};

let fetchMock: Mock;

function mockFetchConfigSuccess(config: ContextConfig = mockConfig) {
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

describe('ContextConfigPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows loading spinner initially', () => {
    mockFetchConfigSuccess();
    render(<ContextConfigPage />);
    expect(screen.getByTestId('context-config-loading')).toBeInTheDocument();
  });

  it('renders config form after loading', async () => {
    mockFetchConfigSuccess();
    render(<ContextConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('context-config-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Context Configuration')).toBeInTheDocument();
    expect(screen.getByTestId('context-config-form')).toBeInTheDocument();

    const maxTokensInput = document.getElementById('max-total-tokens') as HTMLInputElement;
    expect(maxTokensInput.value).toBe('128000');

    // Verify layer budgets table
    expect(screen.getByTestId('layer-budgets-table')).toBeInTheDocument();
    // "system" appears both in the table and in enabled layers chip, so use getAllByText
    expect(screen.getAllByText('system').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('4000')).toBeInTheDocument();
    expect(screen.getAllByText('history').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('32000')).toBeInTheDocument();

    // Verify enabled layers via data-testid
    expect(screen.getByTestId('enabled-layer-rag')).toBeInTheDocument();
    expect(screen.getByTestId('enabled-layer-system')).toBeInTheDocument();
  });

  it('shows error when API fails', async () => {
    mockFetchConfigError();
    render(<ContextConfigPage />);

    await waitFor(() => {
      expect(screen.getByTestId('context-config-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('saves config via PUT with If-Match', async () => {
    mockFetchConfigSuccess();
    render(<ContextConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('context-config-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Modify a field
    const maxTokensInput = document.getElementById('max-total-tokens') as HTMLInputElement;
    await ue.clear(maxTokensInput);
    await ue.type(maxTokensInput, '256000');

    // Mock PUT success and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockConfig, max_total_tokens: 256000 },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-put' },
      }),
    } as Response);
    mockFetchConfigSuccess({ ...mockConfig, max_total_tokens: 256000 });

    await ue.click(screen.getByTestId('save-context-config-btn'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      expect((putCall![0] as string)).toContain('/api/v1/config/context');
      const headers = new Headers((putCall![1] as RequestInit).headers);
      expect(headers.get('If-Match')).toBe('2026-01-01T00:00:00Z');
    });
  });

  it('adds a new layer budget', async () => {
    mockFetchConfigSuccess();
    render(<ContextConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('context-config-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.type(screen.getByTestId('new-layer-key'), 'retrieval');
    await ue.type(screen.getByTestId('new-layer-value'), '16000');
    await ue.click(screen.getByTestId('add-layer-budget-btn'));

    expect(screen.getByText('retrieval')).toBeInTheDocument();
    expect(screen.getByText('16000')).toBeInTheDocument();
  });

  it('adds a new enabled layer', async () => {
    mockFetchConfigSuccess();
    render(<ContextConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('context-config-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();
    await ue.type(screen.getByTestId('new-enabled-layer'), 'retrieval');
    await ue.click(screen.getByTestId('add-enabled-layer-btn'));

    expect(screen.getByTestId('enabled-layer-retrieval')).toBeInTheDocument();
  });

  it('hides save button for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockFetchConfigSuccess();
    render(<ContextConfigPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('context-config-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('save-context-config-btn')).not.toBeInTheDocument();
  });
});
