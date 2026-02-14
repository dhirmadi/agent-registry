import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { SignalsPage } from './SignalsPage';
import type { SignalConfig } from '../types';

let mockUser: { role: string } = { role: 'admin' };
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ user: mockUser }),
}));

const mockAddToast = vi.fn();
vi.mock('../components/ToastNotifications', () => ({
  useToast: () => ({ addToast: mockAddToast }),
}));

const mockSignals: SignalConfig[] = [
  {
    id: 'sig-1',
    source: 'slack',
    poll_interval: '30s',
    is_enabled: true,
    updated_at: '2026-01-01T00:00:00Z',
  },
  {
    id: 'sig-2',
    source: 'github',
    poll_interval: '5m',
    is_enabled: false,
    updated_at: '2026-01-02T00:00:00Z',
  },
];

let fetchMock: Mock;

function mockFetchSignalsSuccess(signals: SignalConfig[] = mockSignals) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: signals, total: signals.length },
      error: null,
      meta: { timestamp: new Date().toISOString(), request_id: 'req-1' },
    }),
  } as Response);
}

function mockFetchSignalsError() {
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

describe('SignalsPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
    mockUser = { role: 'admin' };
  });

  it('shows loading spinner initially', () => {
    mockFetchSignalsSuccess();
    render(<SignalsPage />);
    expect(screen.getByTestId('signals-loading')).toBeInTheDocument();
  });

  it('renders signals table after loading', async () => {
    mockFetchSignalsSuccess();
    render(<SignalsPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('signals-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Signal Polling Configuration')).toBeInTheDocument();
    expect(screen.getByTestId('signals-table')).toBeInTheDocument();
    expect(screen.getByText('slack')).toBeInTheDocument();
    expect(screen.getByText('github')).toBeInTheDocument();
    expect(screen.getByText('30s')).toBeInTheDocument();
    expect(screen.getByText('5m')).toBeInTheDocument();
  });

  it('shows error when API fails', async () => {
    mockFetchSignalsError();
    render(<SignalsPage />);

    await waitFor(() => {
      expect(screen.getByTestId('signals-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('toggles signal enabled status via PATCH', async () => {
    mockFetchSignalsSuccess();
    render(<SignalsPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('signals-loading')).not.toBeInTheDocument();
    });

    // Mock the PATCH call and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockSignals[0], is_enabled: false },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-patch' },
      }),
    } as Response);
    mockFetchSignalsSuccess();

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('toggle-signal-sig-1'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) => (call[1] as RequestInit)?.method === 'PUT',
      );
      expect(putCall).toBeDefined();
      expect((putCall![0] as string)).toContain('/api/v1/signal-config/sig-1');
    });
  });

  it('inline edits poll interval', async () => {
    mockFetchSignalsSuccess();
    render(<SignalsPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('signals-loading')).not.toBeInTheDocument();
    });

    const ue = userEvent.setup();

    // Click on the interval to start editing
    await ue.click(screen.getByTestId('interval-value-sig-1'));

    // Should show input with current value
    const input = screen.getByTestId('edit-interval-sig-1') as HTMLInputElement;
    expect(input.value).toBe('30s');

    // Change the value
    await ue.clear(input);
    await ue.type(input, '1m');

    // Mock PUT and refetch
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { ...mockSignals[0], poll_interval: '1m' },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-put' },
      }),
      headers: new Headers({ 'Content-Type': 'application/json' }),
    } as Response);
    mockFetchSignalsSuccess([{ ...mockSignals[0], poll_interval: '1m' }, mockSignals[1]]);

    await ue.click(screen.getByTestId('save-interval-sig-1'));

    await waitFor(() => {
      const putCall = fetchMock.mock.calls.find(
        (call: unknown[]) =>
          (call[1] as RequestInit)?.method === 'PUT' &&
          (call[0] as string).includes('/api/v1/signal-config/sig-1'),
      );
      expect(putCall).toBeDefined();
      const body = JSON.parse((putCall![1] as RequestInit).body as string);
      expect(body.poll_interval).toBe('1m');
    });
  });

  it('hides action column for viewer role', async () => {
    mockUser = { role: 'viewer' };
    mockFetchSignalsSuccess();
    render(<SignalsPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('signals-loading')).not.toBeInTheDocument();
    });

    expect(screen.queryByTestId('toggle-signal-sig-1')).not.toBeInTheDocument();
  });

  it('shows empty state when no signals exist', async () => {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => ({
        success: true,
        data: { items: [], total: 0 },
        error: null,
        meta: { timestamp: new Date().toISOString(), request_id: 'req-empty' },
      }),
    } as Response);

    render(<SignalsPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('signals-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText(/No signal sources configured/)).toBeInTheDocument();
  });
});
