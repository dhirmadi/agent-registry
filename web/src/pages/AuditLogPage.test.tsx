import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { Mock } from 'vitest';
import { AuditLogPage } from './AuditLogPage';
import type { AuditEntry } from '../types';

const mockEntry1: AuditEntry = {
  id: 1,
  actor: 'admin',
  actor_id: 'usr-1',
  action: 'create',
  resource_type: 'agent',
  resource_id: 'pmo',
  details: { name: 'PMO Agent' },
  ip_address: '127.0.0.1',
  created_at: '2026-02-10T14:30:00Z',
};

const mockEntry2: AuditEntry = {
  id: 2,
  actor: 'editor1',
  actor_id: 'usr-2',
  action: 'update',
  resource_type: 'prompt',
  resource_id: 'prompt-1',
  details: { version: 2 },
  ip_address: '192.168.1.1',
  created_at: '2026-02-11T09:00:00Z',
};

let fetchMock: Mock;

function mockFetchEntries(entries: AuditEntry[], total: number, offset = 0) {
  fetchMock.mockResolvedValueOnce({
    ok: true,
    status: 200,
    json: async () => ({
      success: true,
      data: { items: entries, total, offset, limit: 50 },
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

describe('AuditLogPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    fetchMock = vi.fn();
    globalThis.fetch = fetchMock;
  });

  it('shows a loading spinner initially', () => {
    mockFetchEntries([mockEntry1], 1);
    render(<AuditLogPage />);
    expect(screen.getByTestId('audit-loading')).toBeInTheDocument();
  });

  it('renders audit log table with correct data after loading', async () => {
    mockFetchEntries([mockEntry1, mockEntry2], 2);
    render(<AuditLogPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('audit-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText('Audit Log')).toBeInTheDocument();
    expect(screen.getByTestId('audit-table')).toBeInTheDocument();
    expect(screen.getByTestId('audit-row-1')).toBeInTheDocument();
    expect(screen.getByTestId('audit-row-2')).toBeInTheDocument();
    // Verify specific cell content exists in rows
    const row1 = screen.getByTestId('audit-row-1');
    expect(row1).toHaveTextContent('admin');
    expect(row1).toHaveTextContent('create');
    expect(row1).toHaveTextContent('pmo');
    const row2 = screen.getByTestId('audit-row-2');
    expect(row2).toHaveTextContent('editor1');
    expect(row2).toHaveTextContent('update');
    expect(row2).toHaveTextContent('prompt-1');
  });

  it('shows error alert when API call fails', async () => {
    mockFetchError();
    render(<AuditLogPage />);

    await waitFor(() => {
      expect(screen.getByTestId('audit-error')).toBeInTheDocument();
    });

    expect(screen.getByText(/Server error/)).toBeInTheDocument();
  });

  it('shows pagination info and navigates pages', async () => {
    // First page: 50 entries of 100 total
    const entries = Array.from({ length: 50 }, (_, i) => ({
      ...mockEntry1,
      id: i + 1,
    }));
    mockFetchEntries(entries, 100, 0);
    render(<AuditLogPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('audit-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByTestId('pagination-info')).toHaveTextContent('1–50 of 100');

    // Previous should be disabled on first page
    expect(screen.getByTestId('prev-page')).toBeDisabled();
    // Next should be enabled
    expect(screen.getByTestId('next-page')).not.toBeDisabled();

    // Click next
    const nextEntries = Array.from({ length: 50 }, (_, i) => ({
      ...mockEntry1,
      id: i + 51,
    }));
    mockFetchEntries(nextEntries, 100, 50);

    const ue = userEvent.setup();
    await ue.click(screen.getByTestId('next-page'));

    await waitFor(() => {
      expect(screen.getByTestId('pagination-info')).toHaveTextContent('51–100 of 100');
    });

    // Now previous should be enabled, next disabled
    expect(screen.getByTestId('prev-page')).not.toBeDisabled();
    expect(screen.getByTestId('next-page')).toBeDisabled();
  });

  it('shows empty state when no entries exist', async () => {
    mockFetchEntries([], 0);
    render(<AuditLogPage />);

    await waitFor(() => {
      expect(screen.queryByTestId('audit-loading')).not.toBeInTheDocument();
    });

    expect(screen.getByText(/no audit entries found/i)).toBeInTheDocument();
  });
});
