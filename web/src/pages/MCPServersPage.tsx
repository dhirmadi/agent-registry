import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  EmptyState,
  EmptyStateBody,
  EmptyStateHeader,
  EmptyStateIcon,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Label,
  Modal,
  ModalVariant,
  NumberInput,
  Spinner,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { CubesIcon } from '@patternfly/react-icons';
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table';
import { api } from '../api/client';
import type { MCPServer } from '../types';

interface ServersResponse {
  servers: MCPServer[];
  total: number;
}

interface ServerFormData {
  label: string;
  endpoint: string;
  auth_type: MCPServer['auth_type'];
  auth_credential: string;
  health_endpoint: string;
  circuit_breaker: {
    fail_threshold: number;
    open_duration_s: number;
  };
  discovery_interval: string;
}

const AUTH_TYPE_OPTIONS: MCPServer['auth_type'][] = ['none', 'bearer', 'basic'];

const EMPTY_FORM: ServerFormData = {
  label: '',
  endpoint: '',
  auth_type: 'none',
  auth_credential: '',
  health_endpoint: '',
  circuit_breaker: { fail_threshold: 5, open_duration_s: 30 },
  discovery_interval: '5m',
};

export function MCPServersPage() {
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingServer, setEditingServer] = useState<MCPServer | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<MCPServer | null>(null);
  const [formData, setFormData] = useState<ServerFormData>({ ...EMPTY_FORM });

  const fetchServers = useCallback(async () => {
    try {
      const data = await api.get<ServersResponse>('/api/v1/mcp-servers');
      setServers(data.servers);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load servers');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchServers();
  }, [fetchServers]);

  const openCreateModal = useCallback(() => {
    setEditingServer(null);
    setFormData({ ...EMPTY_FORM });
    setModalOpen(true);
  }, []);

  const openEditModal = useCallback((server: MCPServer) => {
    setEditingServer(server);
    setFormData({
      label: server.label,
      endpoint: server.endpoint,
      auth_type: server.auth_type,
      auth_credential: '',
      health_endpoint: server.health_endpoint,
      circuit_breaker: { ...server.circuit_breaker },
      discovery_interval: server.discovery_interval,
    });
    setModalOpen(true);
  }, []);

  const handleSave = useCallback(async () => {
    try {
      const body = {
        label: formData.label,
        endpoint: formData.endpoint,
        auth_type: formData.auth_type,
        ...(formData.auth_credential ? { auth_credential: formData.auth_credential } : {}),
        health_endpoint: formData.health_endpoint,
        circuit_breaker: formData.circuit_breaker,
        discovery_interval: formData.discovery_interval,
      };

      if (editingServer) {
        await api.put(
          `/api/v1/mcp-servers/${editingServer.id}`,
          body,
          editingServer.updated_at,
        );
      } else {
        await api.post('/api/v1/mcp-servers', body);
      }

      setModalOpen(false);
      setEditingServer(null);
      setFormData({ ...EMPTY_FORM });
      fetchServers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save server');
    }
  }, [editingServer, formData, fetchServers]);

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return;
    try {
      await api.delete(`/api/v1/mcp-servers/${deleteTarget.id}`);
      setDeleteTarget(null);
      fetchServers();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete server');
    }
  }, [deleteTarget, fetchServers]);

  if (loading) {
    return (
      <div data-testid="mcp-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading MCP servers" />
      </div>
    );
  }

  if (error && servers.length === 0) {
    return (
      <Alert variant="danger" title="Error" data-testid="mcp-error">
        {error}
      </Alert>
    );
  }

  if (!loading && servers.length === 0 && !error) {
    return (
      <div>
        <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
          MCP Servers
        </Title>
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="primary" data-testid="create-server-btn" onClick={openCreateModal}>
                Add MCP Server
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
        <EmptyState data-testid="mcp-empty">
          <EmptyStateHeader
            titleText="No MCP servers configured"
            icon={<EmptyStateIcon icon={CubesIcon} />}
            headingLevel="h2"
          />
          <EmptyStateBody>
            Add an MCP server to enable tool integrations for your agents.
          </EmptyStateBody>
        </EmptyState>
      </div>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        MCP Servers
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button variant="primary" data-testid="create-server-btn" onClick={openCreateModal}>
              Add MCP Server
            </Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>

      {error && (
        <Alert
          variant="danger"
          title="Error"
          data-testid="mcp-error"
          style={{ marginBottom: '1rem' }}
        >
          {error}
        </Alert>
      )}

      <Table aria-label="MCP Servers table" data-testid="mcp-table">
        <Thead>
          <Tr>
            <Th>Label</Th>
            <Th>Endpoint</Th>
            <Th>Auth Type</Th>
            <Th>Discovery Interval</Th>
            <Th>Enabled</Th>
            <Th>Actions</Th>
          </Tr>
        </Thead>
        <Tbody>
          {servers.map((server) => (
            <Tr key={server.id} data-testid={`server-row-${server.id}`}>
              <Td>{server.label}</Td>
              <Td>{server.endpoint}</Td>
              <Td>{server.auth_type}</Td>
              <Td>{server.discovery_interval}</Td>
              <Td>
                {server.is_enabled ? (
                  <Label color="green">Enabled</Label>
                ) : (
                  <Label color="grey">Disabled</Label>
                )}
              </Td>
              <Td>
                <Button
                  variant="secondary"
                  size="sm"
                  data-testid="edit-server-btn"
                  onClick={() => openEditModal(server)}
                  style={{ marginRight: '0.5rem' }}
                >
                  Edit
                </Button>
                <Button
                  variant="danger"
                  size="sm"
                  data-testid="delete-server-btn"
                  onClick={() => setDeleteTarget(server)}
                >
                  Delete
                </Button>
              </Td>
            </Tr>
          ))}
        </Tbody>
      </Table>

      {/* Create/Edit Modal */}
      <Modal
        variant={ModalVariant.medium}
        title={editingServer ? 'Edit MCP Server' : 'Add MCP Server'}
        isOpen={modalOpen}
        onClose={() => {
          setModalOpen(false);
          setEditingServer(null);
        }}
        data-testid="server-modal"
        actions={[
          <Button
            key="save"
            variant="primary"
            data-testid="server-modal-save"
            onClick={handleSave}
          >
            Save
          </Button>,
          <Button
            key="cancel"
            variant="link"
            onClick={() => {
              setModalOpen(false);
              setEditingServer(null);
            }}
          >
            Cancel
          </Button>,
        ]}
      >
        <FormGroup label="Label" isRequired fieldId="server-label">
          <TextInput
            id="server-label"
            aria-label="Label"
            value={formData.label}
            onChange={(_event, value) =>
              setFormData((prev) => ({ ...prev, label: value }))
            }
          />
        </FormGroup>
        <FormGroup
          label="Endpoint"
          isRequired
          fieldId="server-endpoint"
          style={{ marginTop: '1rem' }}
        >
          <TextInput
            id="server-endpoint"
            aria-label="Endpoint"
            value={formData.endpoint}
            onChange={(_event, value) =>
              setFormData((prev) => ({ ...prev, endpoint: value }))
            }
          />
        </FormGroup>
        <FormGroup label="Auth Type" fieldId="server-auth-type" style={{ marginTop: '1rem' }}>
          <FormSelect
            id="server-auth-type"
            aria-label="Auth Type"
            value={formData.auth_type}
            onChange={(_event, value) =>
              setFormData((prev) => ({
                ...prev,
                auth_type: value as MCPServer['auth_type'],
              }))
            }
          >
            {AUTH_TYPE_OPTIONS.map((opt) => (
              <FormSelectOption key={opt} value={opt} label={opt} />
            ))}
          </FormSelect>
        </FormGroup>
        {formData.auth_type !== 'none' && (
          <FormGroup
            label="Auth Credential"
            fieldId="server-auth-credential"
            style={{ marginTop: '1rem' }}
          >
            <TextInput
              id="server-auth-credential"
              aria-label="Auth Credential"
              type="password"
              value={formData.auth_credential}
              onChange={(_event, value) =>
                setFormData((prev) => ({ ...prev, auth_credential: value }))
              }
              placeholder={editingServer ? '(unchanged)' : ''}
            />
          </FormGroup>
        )}
        <FormGroup
          label="Health Endpoint"
          fieldId="server-health-endpoint"
          style={{ marginTop: '1rem' }}
        >
          <TextInput
            id="server-health-endpoint"
            aria-label="Health Endpoint"
            value={formData.health_endpoint}
            onChange={(_event, value) =>
              setFormData((prev) => ({ ...prev, health_endpoint: value }))
            }
          />
        </FormGroup>
        <FormGroup
          label="Fail Threshold"
          fieldId="server-fail-threshold"
          style={{ marginTop: '1rem' }}
        >
          <NumberInput
            id="server-fail-threshold"
            aria-label="Fail Threshold"
            value={formData.circuit_breaker.fail_threshold}
            min={1}
            onMinus={() =>
              setFormData((prev) => ({
                ...prev,
                circuit_breaker: {
                  ...prev.circuit_breaker,
                  fail_threshold: Math.max(1, prev.circuit_breaker.fail_threshold - 1),
                },
              }))
            }
            onPlus={() =>
              setFormData((prev) => ({
                ...prev,
                circuit_breaker: {
                  ...prev.circuit_breaker,
                  fail_threshold: prev.circuit_breaker.fail_threshold + 1,
                },
              }))
            }
            onChange={(event: React.FormEvent<HTMLInputElement>) => {
              const val = Number((event.target as HTMLInputElement).value);
              if (!isNaN(val) && val >= 1) {
                setFormData((prev) => ({
                  ...prev,
                  circuit_breaker: { ...prev.circuit_breaker, fail_threshold: val },
                }));
              }
            }}
          />
        </FormGroup>
        <FormGroup
          label="Discovery Interval"
          fieldId="server-discovery-interval"
          style={{ marginTop: '1rem' }}
        >
          <TextInput
            id="server-discovery-interval"
            aria-label="Discovery Interval"
            value={formData.discovery_interval}
            onChange={(_event, value) =>
              setFormData((prev) => ({ ...prev, discovery_interval: value }))
            }
            placeholder="e.g. 5m, 30s, 1h"
          />
        </FormGroup>
      </Modal>

      {/* Delete Confirmation */}
      <Modal
        variant={ModalVariant.small}
        title="Delete MCP Server"
        isOpen={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        data-testid="delete-confirm"
        actions={[
          <Button
            key="confirm"
            variant="danger"
            data-testid="delete-confirm-btn"
            onClick={handleDelete}
          >
            Delete
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setDeleteTarget(null)}>
            Cancel
          </Button>,
        ]}
      >
        Are you sure you want to delete the MCP server{' '}
        <strong>{deleteTarget?.label}</strong>? This action cannot be undone.
      </Modal>
    </div>
  );
}
