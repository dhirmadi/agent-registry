import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Spinner,
  TextInput,
  Title,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
} from '@patternfly/react-table';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { StatusBadge } from '../components/StatusBadge';
import type { SignalConfig } from '../types';

interface SignalsResponse {
  items: SignalConfig[];
  total: number;
}

export function SignalsPage() {
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [signals, setSignals] = useState<SignalConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Inline edit state
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editInterval, setEditInterval] = useState('');

  const fetchSignals = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<SignalsResponse>('/api/v1/config/signals');
      setSignals(data.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load signal configs');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSignals();
  }, [fetchSignals]);

  async function handleToggleEnabled(signal: SignalConfig) {
    try {
      await api.patch(
        `/api/v1/config/signals/${signal.id}`,
        { is_enabled: !signal.is_enabled },
        signal.updated_at,
      );
      await fetchSignals();
    } catch {
      // Error handled by refetch
    }
  }

  function startEditInterval(signal: SignalConfig) {
    setEditingId(signal.id);
    setEditInterval(signal.poll_interval);
  }

  async function saveInterval(signal: SignalConfig) {
    try {
      await api.patch(
        `/api/v1/config/signals/${signal.id}`,
        { poll_interval: editInterval },
        signal.updated_at,
      );
      setEditingId(null);
      await fetchSignals();
    } catch {
      // Error handled by refetch
    }
  }

  function cancelEdit() {
    setEditingId(null);
    setEditInterval('');
  }

  if (loading) {
    return (
      <div data-testid="signals-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading signal configs" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading signal configs" data-testid="signals-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Signal Polling Configuration
      </Title>

      {signals.length === 0 ? (
        <Alert variant="info" title="No signal sources configured" isInline isPlain />
      ) : (
        <Table aria-label="Signal configs table" data-testid="signals-table">
          <Thead>
            <Tr>
              <Th>Source</Th>
              <Th>Poll Interval</Th>
              <Th>Status</Th>
              {canWrite && <Th>Actions</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {signals.map((signal) => (
              <Tr key={signal.id} data-testid={`signal-row-${signal.id}`}>
                <Td dataLabel="Source">{signal.source}</Td>
                <Td dataLabel="Poll Interval">
                  {editingId === signal.id ? (
                    <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                      <TextInput
                        aria-label="Poll interval"
                        value={editInterval}
                        onChange={(_event, val) => setEditInterval(val)}
                        data-testid={`edit-interval-${signal.id}`}
                        style={{ width: '120px' }}
                      />
                      <Button
                        variant="primary"
                        size="sm"
                        onClick={() => saveInterval(signal)}
                        data-testid={`save-interval-${signal.id}`}
                      >
                        Save
                      </Button>
                      <Button variant="link" size="sm" onClick={cancelEdit}>
                        Cancel
                      </Button>
                    </div>
                  ) : (
                    <span
                      style={canWrite ? { cursor: 'pointer', textDecoration: 'underline' } : undefined}
                      onClick={() => canWrite && startEditInterval(signal)}
                      data-testid={`interval-value-${signal.id}`}
                    >
                      {signal.poll_interval}
                    </span>
                  )}
                </Td>
                <Td dataLabel="Status">
                  <StatusBadge status={signal.is_enabled ? 'Enabled' : 'Disabled'} />
                </Td>
                {canWrite && (
                  <Td>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleToggleEnabled(signal)}
                      data-testid={`toggle-signal-${signal.id}`}
                    >
                      {signal.is_enabled ? 'Disable' : 'Enable'}
                    </Button>
                  </Td>
                )}
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}
    </div>
  );
}
