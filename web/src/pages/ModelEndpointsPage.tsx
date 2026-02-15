import { useEffect, useState, useMemo, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Label,
  Modal,
  ModalVariant,
  SearchInput,
  Spinner,
  Switch,
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
import { useToast } from '../components/ToastNotifications';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import type { ModelEndpoint, ModelProvider } from '../types';

interface EndpointsListResponse {
  endpoints: ModelEndpoint[];
  total: number;
}

const PROVIDER_OPTIONS: ModelProvider[] = ['openai', 'azure', 'anthropic', 'ollama', 'custom'];

const PROVIDER_COLORS: Record<ModelProvider, 'blue' | 'green' | 'orange' | 'purple' | 'grey'> = {
  openai: 'blue',
  azure: 'green',
  anthropic: 'orange',
  ollama: 'purple',
  custom: 'grey',
};

export function ModelEndpointsPage() {
  const navigate = useNavigate();
  const { user } = useAuth();
  const { addToast } = useToast();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [endpoints, setEndpoints] = useState<ModelEndpoint[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nameFilter, setNameFilter] = useState('');

  // Delete confirmation
  const [confirmDelete, setConfirmDelete] = useState<ModelEndpoint | null>(null);

  // Create modal
  const [createOpen, setCreateOpen] = useState(false);
  const [createSlug, setCreateSlug] = useState('');
  const [createName, setCreateName] = useState('');
  const [createProvider, setCreateProvider] = useState<ModelProvider>('openai');
  const [createEndpointUrl, setCreateEndpointUrl] = useState('');
  const [createIsFixedModel, setCreateIsFixedModel] = useState(true);
  const [createModelName, setCreateModelName] = useState('');
  const [createAllowedModels, setCreateAllowedModels] = useState('');
  const [createError, setCreateError] = useState<string | null>(null);

  const fetchEndpoints = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<EndpointsListResponse>('/api/v1/model-endpoints');
      setEndpoints(data.endpoints ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load model endpoints');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchEndpoints();
  }, [fetchEndpoints]);

  const filteredEndpoints = useMemo(() => {
    if (!nameFilter) return endpoints;
    const lower = nameFilter.toLowerCase();
    return endpoints.filter(
      (e) =>
        e.name.toLowerCase().includes(lower) ||
        e.slug.toLowerCase().includes(lower),
    );
  }, [endpoints, nameFilter]);

  async function handleDelete(endpoint: ModelEndpoint) {
    try {
      await api.delete(`/api/v1/model-endpoints/${endpoint.slug}`);
      await fetchEndpoints();
    } catch (err) {
      addToast('danger', 'Operation failed', err instanceof Error ? err.message : 'An unknown error occurred');
    }
    setConfirmDelete(null);
  }

  async function handleCreate() {
    setCreateError(null);
    try {
      const body: Record<string, unknown> = {
        slug: createSlug,
        name: createName,
        provider: createProvider,
        endpoint_url: createEndpointUrl,
        is_fixed_model: createIsFixedModel,
        model_name: createModelName,
      };
      if (!createIsFixedModel && createAllowedModels.trim()) {
        body.allowed_models = createAllowedModels.split(',').map((s) => s.trim()).filter(Boolean);
      }
      await api.post('/api/v1/model-endpoints', body);
      setCreateOpen(false);
      resetCreateForm();
      await fetchEndpoints();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create endpoint');
    }
  }

  function resetCreateForm() {
    setCreateSlug('');
    setCreateName('');
    setCreateProvider('openai');
    setCreateEndpointUrl('');
    setCreateIsFixedModel(true);
    setCreateModelName('');
    setCreateAllowedModels('');
    setCreateError(null);
  }

  function getRowActions(endpoint: ModelEndpoint): IAction[] {
    return [
      {
        title: 'Delete',
        onClick: () => setConfirmDelete(endpoint),
      },
    ];
  }

  if (loading) {
    return (
      <div data-testid="endpoints-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading model endpoints" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading model endpoints" data-testid="endpoints-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Model Endpoints
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <SearchInput
              placeholder="Filter by name or slug"
              value={nameFilter}
              onChange={(_event, value) => setNameFilter(value)}
              onClear={() => setNameFilter('')}
            />
          </ToolbarItem>
          {canWrite && (
            <ToolbarItem align={{ default: 'alignRight' }}>
              <Button variant="primary" onClick={() => setCreateOpen(true)} data-testid="create-endpoint-btn">
                Create Endpoint
              </Button>
            </ToolbarItem>
          )}
        </ToolbarContent>
      </Toolbar>

      {filteredEndpoints.length === 0 ? (
        <Alert variant="info" title="No model endpoints found" isInline isPlain data-testid="endpoints-empty" />
      ) : (
        <Table aria-label="Model endpoints table" data-testid="endpoints-table">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Slug</Th>
              <Th>Provider</Th>
              <Th>Model</Th>
              <Th>Version</Th>
              <Th>Status</Th>
              <Th>Last Updated</Th>
              {canWrite && <Th screenReaderText="Actions" />}
            </Tr>
          </Thead>
          <Tbody>
            {filteredEndpoints.map((endpoint) => (
              <Tr
                key={endpoint.id}
                isClickable
                onRowClick={() => navigate(`/model-endpoints/${endpoint.slug}`)}
              >
                <Td dataLabel="Name">{endpoint.name}</Td>
                <Td dataLabel="Slug">{endpoint.slug}</Td>
                <Td dataLabel="Provider">
                  <Label color={PROVIDER_COLORS[endpoint.provider] ?? 'grey'}>
                    {endpoint.provider}
                  </Label>
                </Td>
                <Td dataLabel="Model">
                  {endpoint.is_fixed_model
                    ? endpoint.model_name
                    : `${endpoint.model_name} (+${endpoint.allowed_models?.length ?? 0})`}
                </Td>
                <Td dataLabel="Version">v{endpoint.version}</Td>
                <Td dataLabel="Status">
                  <StatusBadge status={endpoint.is_active ? 'Active' : 'Inactive'} />
                </Td>
                <Td dataLabel="Last Updated">
                  {new Date(endpoint.updated_at).toLocaleDateString()}
                </Td>
                {canWrite && (
                  <Td isActionCell>
                    <div data-testid={`endpoint-actions-${endpoint.slug}`}>
                      <ActionsColumn items={getRowActions(endpoint)} />
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
        title="Delete Model Endpoint"
        message={`Are you sure you want to delete endpoint "${confirmDelete?.name}"? This action cannot be undone.`}
        confirmText="Delete"
        variant="danger"
        onConfirm={() => confirmDelete && handleDelete(confirmDelete)}
        onCancel={() => setConfirmDelete(null)}
      />

      <Modal
        isOpen={createOpen}
        onClose={() => {
          setCreateOpen(false);
          resetCreateForm();
        }}
        title="Create Model Endpoint"
        variant={ModalVariant.medium}
        aria-label="Create Model Endpoint"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreate}
            isDisabled={!createSlug || !createName || !createEndpointUrl || !createModelName}
            data-testid="create-endpoint-submit"
          >
            Create
          </Button>,
          <Button
            key="cancel"
            variant="link"
            onClick={() => {
              setCreateOpen(false);
              resetCreateForm();
            }}
          >
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
          <FormGroup label="Slug" isRequired fieldId="create-endpoint-slug">
            <TextInput
              id="create-endpoint-slug"
              value={createSlug}
              onChange={(_event, val) => setCreateSlug(val)}
              placeholder="e.g., openai-gpt4o-prod"
            />
          </FormGroup>
          <FormGroup label="Name" isRequired fieldId="create-endpoint-name">
            <TextInput
              id="create-endpoint-name"
              value={createName}
              onChange={(_event, val) => setCreateName(val)}
              placeholder="e.g., GPT-4o Production"
            />
          </FormGroup>
          <FormGroup label="Provider" isRequired fieldId="create-endpoint-provider">
            <FormSelect
              id="create-endpoint-provider"
              value={createProvider}
              onChange={(_event, val) => setCreateProvider(val as ModelProvider)}
            >
              {PROVIDER_OPTIONS.map((p) => (
                <FormSelectOption key={p} value={p} label={p} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Endpoint URL" isRequired fieldId="create-endpoint-url">
            <TextInput
              id="create-endpoint-url"
              value={createEndpointUrl}
              onChange={(_event, val) => setCreateEndpointUrl(val)}
              placeholder="e.g., https://api.openai.com/v1"
            />
          </FormGroup>
          <FormGroup label="Fixed Model" fieldId="create-endpoint-fixed">
            <Switch
              id="create-endpoint-fixed"
              isChecked={createIsFixedModel}
              onChange={(_event, checked) => setCreateIsFixedModel(checked)}
              label="Fixed model (consumers use this model only)"
              labelOff="Flexible (consumers select from allowed list)"
            />
          </FormGroup>
          <FormGroup label="Model Name" isRequired fieldId="create-endpoint-model">
            <TextInput
              id="create-endpoint-model"
              value={createModelName}
              onChange={(_event, val) => setCreateModelName(val)}
              placeholder="e.g., gpt-4o-2024-08-06"
            />
          </FormGroup>
          {!createIsFixedModel && (
            <FormGroup label="Allowed Models" fieldId="create-endpoint-allowed">
              <TextArea
                id="create-endpoint-allowed"
                value={createAllowedModels}
                onChange={(_event, val) => setCreateAllowedModels(val)}
                placeholder="Comma-separated: gpt-4o, gpt-4o-mini, gpt-4-turbo"
                rows={2}
              />
            </FormGroup>
          )}
        </Form>
      </Modal>
    </div>
  );
}
