import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Spinner,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import {
  Table,
  Thead,
  Tr,
  Th,
  Tbody,
  Td,
  ExpandableRowContent,
} from '@patternfly/react-table';
import type { AuditEntry, PaginatedResponse } from '../types';
import { api } from '../api/client';

const PAGE_SIZE = 50;

const RESOURCE_TYPES = ['', 'agent', 'prompt', 'mcp_server', 'trust', 'trigger', 'user', 'webhook', 'api_key', 'config'];
const ACTIONS = ['', 'create', 'update', 'delete', 'activate', 'deactivate', 'login', 'logout'];

export function AuditLogPage() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [offset, setOffset] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [actorFilter, setActorFilter] = useState('');
  const [resourceTypeFilter, setResourceTypeFilter] = useState('');
  const [actionFilter, setActionFilter] = useState('');

  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());

  const fetchEntries = useCallback(async (currentOffset: number) => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams({
        offset: String(currentOffset),
        limit: String(PAGE_SIZE),
      });
      if (actorFilter) params.set('actor', actorFilter);
      if (resourceTypeFilter) params.set('resource_type', resourceTypeFilter);
      if (actionFilter) params.set('action', actionFilter);

      const data = await api.get<PaginatedResponse<AuditEntry>>(
        `/api/v1/audit-log?${params.toString()}`,
      );
      setEntries(data.items ?? []);
      setTotal(data.total);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load audit log');
    } finally {
      setLoading(false);
    }
  }, [actorFilter, resourceTypeFilter, actionFilter]);

  useEffect(() => {
    fetchEntries(offset);
  }, [fetchEntries, offset]);

  function handleApplyFilters() {
    setOffset(0);
    fetchEntries(0);
  }

  function toggleRow(id: number) {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }

  const hasPrev = offset > 0;
  const hasNext = offset + PAGE_SIZE < total;

  if (loading && entries.length === 0) {
    return (
      <div data-testid="audit-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading audit log" />
      </div>
    );
  }

  if (error && entries.length === 0) {
    return (
      <Alert variant="danger" title="Error loading audit log" data-testid="audit-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Audit Log
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <FormGroup label="Actor" fieldId="filter-actor">
              <TextInput
                id="filter-actor"
                value={actorFilter}
                onChange={(_event, val) => setActorFilter(val)}
                placeholder="Filter by actor"
              />
            </FormGroup>
          </ToolbarItem>
          <ToolbarItem>
            <FormGroup label="Resource Type" fieldId="filter-resource-type">
              <FormSelect
                id="filter-resource-type"
                value={resourceTypeFilter}
                onChange={(_event, val) => setResourceTypeFilter(val)}
              >
                {RESOURCE_TYPES.map((rt) => (
                  <FormSelectOption key={rt} value={rt} label={rt || 'All'} />
                ))}
              </FormSelect>
            </FormGroup>
          </ToolbarItem>
          <ToolbarItem>
            <FormGroup label="Action" fieldId="filter-action">
              <FormSelect
                id="filter-action"
                value={actionFilter}
                onChange={(_event, val) => setActionFilter(val)}
              >
                {ACTIONS.map((a) => (
                  <FormSelectOption key={a} value={a} label={a || 'All'} />
                ))}
              </FormSelect>
            </FormGroup>
          </ToolbarItem>
          <ToolbarItem>
            <Button
              variant="secondary"
              onClick={handleApplyFilters}
              style={{ marginTop: '1.5rem' }}
              data-testid="apply-filters"
            >
              Apply Filters
            </Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>

      {entries.length === 0 && !loading ? (
        <Alert variant="info" title="No audit entries found" isInline isPlain />
      ) : (
        <Table aria-label="Audit log table" data-testid="audit-table">
          <Thead>
            <Tr>
              <Th screenReaderText="Expand" />
              <Th>Timestamp</Th>
              <Th>Actor</Th>
              <Th>Action</Th>
              <Th>Resource Type</Th>
              <Th>Resource ID</Th>
            </Tr>
          </Thead>
          <Tbody>
            {entries.map((entry) => {
              const isExpanded = expandedRows.has(entry.id);
              return [
                <Tr key={entry.id} data-testid={`audit-row-${entry.id}`}>
                  <Td
                    expand={{
                      rowIndex: entry.id,
                      isExpanded,
                      onToggle: () => toggleRow(entry.id),
                    }}
                  />
                  <Td dataLabel="Timestamp">
                    {new Date(entry.created_at).toLocaleString()}
                  </Td>
                  <Td dataLabel="Actor">{entry.actor}</Td>
                  <Td dataLabel="Action">{entry.action}</Td>
                  <Td dataLabel="Resource Type">{entry.resource_type}</Td>
                  <Td dataLabel="Resource ID">{entry.resource_id}</Td>
                </Tr>,
                <Tr key={`${entry.id}-detail`} isExpanded={isExpanded}>
                  <Td colSpan={6}>
                    <ExpandableRowContent>
                      <pre style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
                        {JSON.stringify(entry.details, null, 2)}
                      </pre>
                    </ExpandableRowContent>
                  </Td>
                </Tr>,
              ];
            })}
          </Tbody>
        </Table>
      )}

      <Toolbar style={{ marginTop: '1rem' }}>
        <ToolbarContent>
          <ToolbarItem>
            <Button
              variant="secondary"
              isDisabled={!hasPrev}
              onClick={() => setOffset((prev) => Math.max(0, prev - PAGE_SIZE))}
              data-testid="prev-page"
            >
              Previous
            </Button>
          </ToolbarItem>
          <ToolbarItem>
            <span data-testid="pagination-info">
              {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
            </span>
          </ToolbarItem>
          <ToolbarItem>
            <Button
              variant="secondary"
              isDisabled={!hasNext}
              onClick={() => setOffset((prev) => prev + PAGE_SIZE)}
              data-testid="next-page"
            >
              Next
            </Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>
    </div>
  );
}
