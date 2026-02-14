import { useEffect, useState, useMemo, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  Modal,
  SearchInput,
  Spinner,
  TextArea,
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
  ActionsColumn,
} from '@patternfly/react-table';
import type { IAction } from '@patternfly/react-table';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import type { Agent } from '../types';

interface AgentsListResponse {
  items: Agent[];
  total: number;
}

export function AgentsPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nameFilter, setNameFilter] = useState('');

  // Delete confirmation
  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  // Create modal
  const [createOpen, setCreateOpen] = useState(false);
  const [createId, setCreateId] = useState('');
  const [createName, setCreateName] = useState('');
  const [createDesc, setCreateDesc] = useState('');
  const [createError, setCreateError] = useState<string | null>(null);

  const fetchAgents = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<AgentsListResponse>('/api/v1/agents?active_only=false');
      setAgents(data.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agents');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();
  }, [fetchAgents]);

  const filteredAgents = useMemo(() => {
    if (!nameFilter) return agents;
    const lower = nameFilter.toLowerCase();
    return agents.filter((a) => a.name.toLowerCase().includes(lower));
  }, [agents, nameFilter]);

  async function handleDelete(agentId: string) {
    try {
      await api.delete(`/api/v1/agents/${agentId}`);
      await fetchAgents();
    } catch {
      // Error handled by refetch
    }
    setConfirmDelete(null);
  }

  async function handleToggleActive(agent: Agent) {
    try {
      await api.patch(`/api/v1/agents/${agent.id}`, { is_active: !agent.is_active }, agent.updated_at);
      await fetchAgents();
    } catch {
      // Error handled by refetch
    }
  }

  async function handleCreate() {
    setCreateError(null);
    try {
      await api.post('/api/v1/agents', {
        id: createId,
        name: createName,
        description: createDesc,
      });
      setCreateOpen(false);
      setCreateId('');
      setCreateName('');
      setCreateDesc('');
      await fetchAgents();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create agent');
    }
  }

  if (loading) {
    return (
      <div data-testid="agents-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading agents" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading agents" data-testid="agents-error">
        {error}
      </Alert>
    );
  }

  function getRowActions(agent: Agent): IAction[] {
    return [
      {
        title: agent.is_active ? 'Deactivate' : 'Activate',
        onClick: () => handleToggleActive(agent),
      },
      {
        title: 'Delete',
        onClick: () => setConfirmDelete(agent.id),
      },
    ];
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Agents
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <SearchInput
              placeholder="Filter by name"
              value={nameFilter}
              onChange={(_event, value) => setNameFilter(value)}
              onClear={() => setNameFilter('')}
            />
          </ToolbarItem>
          {canWrite && (
            <ToolbarItem align={{ default: 'alignRight' }}>
              <Button variant="primary" onClick={() => setCreateOpen(true)}>
                Create Agent
              </Button>
            </ToolbarItem>
          )}
        </ToolbarContent>
      </Toolbar>

      {filteredAgents.length === 0 ? (
        <Alert variant="info" title="No agents found" isInline isPlain />
      ) : (
        <Table aria-label="Agents table">
          <Thead>
            <Tr>
              <Th>ID</Th>
              <Th>Name</Th>
              <Th>Version</Th>
              <Th>Tools</Th>
              <Th>Status</Th>
              <Th>Last Updated</Th>
              {canWrite && <Th screenReaderText="Actions" />}
            </Tr>
          </Thead>
          <Tbody>
            {filteredAgents.map((agent) => (
              <Tr
                key={agent.id}
                isClickable
                onRowClick={() => navigate(`/agents/${agent.id}`)}
              >
                <Td dataLabel="ID">{agent.id}</Td>
                <Td dataLabel="Name">{agent.name}</Td>
                <Td dataLabel="Version">{agent.version}</Td>
                <Td dataLabel="Tools">{agent.tools.length}</Td>
                <Td dataLabel="Status">
                  <StatusBadge status={agent.is_active ? 'Active' : 'Inactive'} />
                </Td>
                <Td dataLabel="Last Updated">
                  {new Date(agent.updated_at).toLocaleDateString()}
                </Td>
                {canWrite && (
                  <Td isActionCell>
                    <div data-testid={`agent-actions-${agent.id}`}>
                      <ActionsColumn items={getRowActions(agent)} />
                    </div>
                  </Td>
                )}
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <ConfirmDialog
        isOpen={confirmDelete !== null}
        title="Delete Agent"
        message={`Are you sure you want to delete agent "${confirmDelete}"? This will deactivate the agent.`}
        confirmText="Delete"
        variant="danger"
        onConfirm={() => confirmDelete && handleDelete(confirmDelete)}
        onCancel={() => setConfirmDelete(null)}
      />

      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Create New Agent"
        variant="medium"
        aria-label="Create New Agent"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreate}
            isDisabled={!createId || !createName}
            data-testid="create-agent-submit"
          >
            Create
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setCreateOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        {createError && (
          <Alert variant="danger" title="Error" isInline style={{ marginBottom: '1rem' }}>
            {createError}
          </Alert>
        )}
        <Form>
          <FormGroup label="Agent ID" isRequired fieldId="create-agent-id">
            <TextInput
              id="create-agent-id"
              value={createId}
              onChange={(_event, val) => setCreateId(val)}
              placeholder="e.g., knowledge-steward"
            />
          </FormGroup>
          <FormGroup label="Name" isRequired fieldId="create-agent-name">
            <TextInput
              id="create-agent-name"
              value={createName}
              onChange={(_event, val) => setCreateName(val)}
              placeholder="e.g., Knowledge Steward"
            />
          </FormGroup>
          <FormGroup label="Description" fieldId="create-agent-desc">
            <TextArea
              id="create-agent-desc"
              value={createDesc}
              onChange={(_event, val) => setCreateDesc(val)}
              placeholder="Agent description"
              rows={3}
            />
          </FormGroup>
        </Form>
      </Modal>
    </div>
  );
}
