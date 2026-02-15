import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Spinner,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { api } from '../api/client';
import { StatusBadge } from '../components/StatusBadge';
import type { AuditEntry, PaginatedResponse } from '../types';

interface GatewayServer {
  label: string;
  endpoint: string;
}

interface GatewayToolsResponse {
  servers: GatewayServer[];
}

export function GatewayPage() {
  const [servers, setServers] = useState<GatewayServer[]>([]);
  const [auditEntries, setAuditEntries] = useState<AuditEntry[]>([]);
  const [loadingTools, setLoadingTools] = useState(true);
  const [loadingAudit, setLoadingAudit] = useState(true);
  const [toolsError, setToolsError] = useState<string | null>(null);
  const [auditError, setAuditError] = useState<string | null>(null);

  const fetchTools = useCallback(async () => {
    setLoadingTools(true);
    setToolsError(null);
    try {
      const data = await api.get<GatewayToolsResponse>('/mcp/v1/tools');
      setServers(data.servers ?? []);
    } catch (err) {
      setToolsError(err instanceof Error ? err.message : 'Failed to load gateway tools');
    } finally {
      setLoadingTools(false);
    }
  }, []);

  const fetchAudit = useCallback(async () => {
    setLoadingAudit(true);
    setAuditError(null);
    try {
      const data = await api.get<PaginatedResponse<AuditEntry>>(
        '/api/v1/audit-log?action=gateway_tool_call&limit=20',
      );
      setAuditEntries(data.items ?? []);
    } catch (err) {
      setAuditError(err instanceof Error ? err.message : 'Failed to load gateway audit log');
    } finally {
      setLoadingAudit(false);
    }
  }, []);

  useEffect(() => {
    fetchTools();
    fetchAudit();
  }, [fetchTools, fetchAudit]);

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Gateway
      </Title>

      <Title headingLevel="h2" size="lg" style={{ marginBottom: '1rem' }}>
        MCP Servers
      </Title>

      {loadingTools ? (
        <div data-testid="tools-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading gateway tools" />
        </div>
      ) : toolsError ? (
        <Alert variant="danger" title="Error loading gateway tools" data-testid="tools-error">
          {toolsError}
        </Alert>
      ) : servers.length === 0 ? (
        <Alert variant="info" title="No gateway servers available" isInline isPlain data-testid="tools-empty" />
      ) : (
        <Table aria-label="Gateway servers table" data-testid="tools-table">
          <Thead>
            <Tr>
              <Th>Label</Th>
              <Th>Endpoint</Th>
              <Th>Status</Th>
            </Tr>
          </Thead>
          <Tbody>
            {servers.map((server) => (
              <Tr key={server.label} data-testid={`server-row-${server.label}`}>
                <Td dataLabel="Label">{server.label}</Td>
                <Td dataLabel="Endpoint">{server.endpoint}</Td>
                <Td dataLabel="Status">
                  <StatusBadge status="Enabled" />
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <Title headingLevel="h2" size="lg" style={{ marginTop: '2rem', marginBottom: '1rem' }}>
        Recent Gateway Calls
      </Title>

      {loadingAudit ? (
        <div data-testid="audit-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading gateway audit log" />
        </div>
      ) : auditError ? (
        <Alert variant="danger" title="Error loading gateway audit log" data-testid="audit-error">
          {auditError}
        </Alert>
      ) : auditEntries.length === 0 ? (
        <Alert variant="info" title="No recent gateway calls" isInline isPlain data-testid="audit-empty" />
      ) : (
        <Table aria-label="Gateway audit log table" data-testid="audit-table">
          <Thead>
            <Tr>
              <Th>Timestamp</Th>
              <Th>Actor</Th>
              <Th>Server / Tool</Th>
              <Th>Outcome</Th>
              <Th>Latency</Th>
            </Tr>
          </Thead>
          <Tbody>
            {auditEntries.map((entry) => {
              const details = entry.details ?? {};
              return (
                <Tr key={entry.id} data-testid={`audit-row-${entry.id}`}>
                  <Td dataLabel="Timestamp">
                    {new Date(entry.created_at).toLocaleString()}
                  </Td>
                  <Td dataLabel="Actor">{entry.actor}</Td>
                  <Td dataLabel="Server / Tool">{entry.resource_id}</Td>
                  <Td dataLabel="Outcome">
                    <StatusBadge
                      status={
                        (details.outcome as string) === 'success' ? 'Healthy' :
                        (details.outcome as string) ?? 'unknown'
                      }
                    />
                  </Td>
                  <Td dataLabel="Latency">
                    {details.latency_ms != null ? `${details.latency_ms}ms` : '-'}
                  </Td>
                </Tr>
              );
            })}
          </Tbody>
        </Table>
      )}
    </div>
  );
}
