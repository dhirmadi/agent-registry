import { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
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
  Spinner,
  Switch,
  Tab,
  TabContentBody,
  TabTitleText,
  Tabs,
  TextArea,
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
import { api, APIError } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { useToast } from '../components/ToastNotifications';
import { StatusBadge } from '../components/StatusBadge';
import { VersionTimeline } from '../components/VersionTimeline';
import type { TimelineVersion } from '../components/VersionTimeline';
import { JsonEditor } from '../components/JsonEditor';
import type { ModelEndpoint, ModelEndpointVersion, ModelProvider } from '../types';

interface VersionsResponse {
  versions: ModelEndpointVersion[];
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

export function ModelEndpointDetailPage() {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();
  const { addToast } = useToast();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [endpoint, setEndpoint] = useState<ModelEndpoint | null>(null);
  const [versions, setVersions] = useState<ModelEndpointVersion[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState(0);
  const [conflictMessage, setConflictMessage] = useState<string | null>(null);

  // Editable form state (Overview tab)
  const [editName, setEditName] = useState('');
  const [editProvider, setEditProvider] = useState<ModelProvider>('openai');
  const [editEndpointUrl, setEditEndpointUrl] = useState('');
  const [editIsFixedModel, setEditIsFixedModel] = useState(true);
  const [editModelName, setEditModelName] = useState('');
  const [editAllowedModels, setEditAllowedModels] = useState('');

  // Create version modal
  const [versionModalOpen, setVersionModalOpen] = useState(false);
  const [newConfigJson, setNewConfigJson] = useState('{\n  "temperature": 0.7,\n  "max_tokens": 4096\n}');
  const [newChangeNote, setNewChangeNote] = useState('');
  const [versionError, setVersionError] = useState<string | null>(null);

  const fetchData = useCallback(async () => {
    if (!slug) return;
    setLoading(true);
    setError(null);
    try {
      const [endpointData, versionsData] = await Promise.all([
        api.get<ModelEndpoint>(`/api/v1/model-endpoints/${slug}`),
        api.get<VersionsResponse>(`/api/v1/model-endpoints/${slug}/versions`),
      ]);
      setEndpoint(endpointData);
      setVersions(versionsData.versions ?? []);
      // Populate form state
      setEditName(endpointData.name);
      setEditProvider(endpointData.provider);
      setEditEndpointUrl(endpointData.endpoint_url);
      setEditIsFixedModel(endpointData.is_fixed_model);
      setEditModelName(endpointData.model_name);
      setEditAllowedModels((endpointData.allowed_models ?? []).join(', '));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load endpoint');
    } finally {
      setLoading(false);
    }
  }, [slug]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  async function handleSaveOverview() {
    if (!endpoint) return;
    setConflictMessage(null);
    try {
      const body: Record<string, unknown> = {
        name: editName,
        provider: editProvider,
        endpoint_url: editEndpointUrl,
        is_fixed_model: editIsFixedModel,
        model_name: editModelName,
      };
      if (!editIsFixedModel && editAllowedModels.trim()) {
        body.allowed_models = editAllowedModels.split(',').map((s) => s.trim()).filter(Boolean);
      } else {
        body.allowed_models = [];
      }

      await api.put(`/api/v1/model-endpoints/${endpoint.slug}`, body, endpoint.updated_at);
      addToast('success', 'Endpoint saved');
      await fetchData();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        setConflictMessage(err.message);
      } else {
        addToast('danger', 'Save failed', err instanceof Error ? err.message : 'Unknown error');
      }
    }
  }

  async function handleCreateVersion() {
    if (!endpoint) return;
    setVersionError(null);
    try {
      const config = JSON.parse(newConfigJson);
      await api.post(`/api/v1/model-endpoints/${endpoint.slug}/versions`, {
        config,
        change_note: newChangeNote,
      });
      setVersionModalOpen(false);
      setNewConfigJson('{\n  "temperature": 0.7,\n  "max_tokens": 4096\n}');
      setNewChangeNote('');
      addToast('success', 'Version created');
      await fetchData();
    } catch (err) {
      if (err instanceof SyntaxError) {
        setVersionError('Invalid JSON in config');
      } else {
        setVersionError(err instanceof Error ? err.message : 'Failed to create version');
      }
    }
  }

  async function handleActivateVersion(version: number) {
    if (!endpoint) return;
    try {
      await api.post(`/api/v1/model-endpoints/${endpoint.slug}/versions/${version}/activate`);
      addToast('success', 'Version activated', `Version ${version} is now active`);
      await fetchData();
    } catch (err) {
      addToast('danger', 'Activation failed', err instanceof Error ? err.message : 'Unknown error');
    }
  }

  if (loading) {
    return (
      <div data-testid="endpoint-detail-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading endpoint" />
      </div>
    );
  }

  if (error || !endpoint) {
    return (
      <Alert variant="danger" title="Error" data-testid="endpoint-detail-error">
        {error ?? 'Endpoint not found'}
      </Alert>
    );
  }

  const activeVersion = versions.find((v) => v.is_active);

  const timelineVersions: TimelineVersion[] = versions.map((v) => ({
    version: v.version,
    created_by: v.created_by,
    created_at: v.created_at,
    is_current: v.is_active,
  }));

  return (
    <div>
      <div style={{ marginBottom: '1rem' }}>
        <Button variant="link" onClick={() => navigate('/model-endpoints')}>
          Back to Model Endpoints
        </Button>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.5rem' }}>
        <Title headingLevel="h1">{endpoint.name}</Title>
        <StatusBadge status={endpoint.is_active ? 'Active' : 'Inactive'} />
        <Label color={PROVIDER_COLORS[endpoint.provider] ?? 'grey'}>{endpoint.provider}</Label>
        <Label color="blue">v{endpoint.version}</Label>
        <Label>{endpoint.slug}</Label>
      </div>

      {conflictMessage && (
        <Alert
          variant="warning"
          title="Conflict"
          isInline
          style={{ marginBottom: '1rem' }}
        >
          {conflictMessage}. Please reload the page and try again.
        </Alert>
      )}

      <Tabs
        activeKey={activeTab}
        onSelect={(_event, tabIndex) => setActiveTab(tabIndex as number)}
      >
        <Tab eventKey={0} title={<TabTitleText>Overview</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            <Form>
              <FormGroup label="Name" fieldId="edit-name">
                <TextInput
                  id="edit-name"
                  value={editName}
                  onChange={(_event, val) => setEditName(val)}
                  isDisabled={!canWrite}
                />
              </FormGroup>
              <FormGroup label="Provider" fieldId="edit-provider">
                <FormSelect
                  id="edit-provider"
                  value={editProvider}
                  onChange={(_event, val) => setEditProvider(val as ModelProvider)}
                  isDisabled={!canWrite}
                >
                  {PROVIDER_OPTIONS.map((p) => (
                    <FormSelectOption key={p} value={p} label={p} />
                  ))}
                </FormSelect>
              </FormGroup>
              <FormGroup label="Endpoint URL" fieldId="edit-endpoint-url">
                <TextInput
                  id="edit-endpoint-url"
                  value={editEndpointUrl}
                  onChange={(_event, val) => setEditEndpointUrl(val)}
                  isDisabled={!canWrite}
                />
              </FormGroup>
              <FormGroup label="Fixed Model" fieldId="edit-fixed-model">
                <Switch
                  id="edit-fixed-model"
                  isChecked={editIsFixedModel}
                  onChange={(_event, checked) => setEditIsFixedModel(checked)}
                  isDisabled={!canWrite}
                  label="Fixed model"
                  labelOff="Flexible"
                />
              </FormGroup>
              <FormGroup label="Model Name" fieldId="edit-model-name">
                <TextInput
                  id="edit-model-name"
                  value={editModelName}
                  onChange={(_event, val) => setEditModelName(val)}
                  isDisabled={!canWrite}
                />
              </FormGroup>
              {!editIsFixedModel && (
                <FormGroup label="Allowed Models" fieldId="edit-allowed-models">
                  <TextArea
                    id="edit-allowed-models"
                    value={editAllowedModels}
                    onChange={(_event, val) => setEditAllowedModels(val)}
                    isDisabled={!canWrite}
                    rows={2}
                    placeholder="Comma-separated model names"
                  />
                </FormGroup>
              )}
              {canWrite && (
                <Button
                  variant="primary"
                  onClick={handleSaveOverview}
                  data-testid="save-overview"
                >
                  Save
                </Button>
              )}
            </Form>
          </TabContentBody>
        </Tab>

        <Tab eventKey={1} title={<TabTitleText>Configuration</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            {activeVersion ? (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginBottom: '1rem' }}>
                  <strong>Active Version: v{activeVersion.version}</strong>
                  {activeVersion.change_note && (
                    <span style={{ color: 'var(--pf-t--global--text--color--subtle, #6a6e73)' }}>
                      — {activeVersion.change_note}
                    </span>
                  )}
                </div>
                <JsonEditor
                  value={JSON.stringify(activeVersion.config, null, 2)}
                  onChange={() => {}}
                  label="Active Configuration"
                  readOnly
                />
              </>
            ) : (
              <Alert variant="info" title="No active version" isInline isPlain>
                Create a version to configure this endpoint.
              </Alert>
            )}
            {canWrite && (
              <Button
                variant="primary"
                onClick={() => setVersionModalOpen(true)}
                style={{ marginTop: '1rem' }}
                data-testid="create-version-btn"
              >
                Create New Version
              </Button>
            )}
          </TabContentBody>
        </Tab>

        <Tab eventKey={2} title={<TabTitleText>Version History</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            {versions.length === 0 ? (
              <Alert variant="info" title="No version history" isInline isPlain />
            ) : (
              <>
                <VersionTimeline
                  versions={timelineVersions}
                  onRollback={handleActivateVersion}
                  readOnly={!canWrite}
                />

                <Title headingLevel="h3" style={{ marginTop: '2rem', marginBottom: '1rem' }}>
                  Version Details
                </Title>
                <Table aria-label="Version details">
                  <Thead>
                    <Tr>
                      <Th>Version</Th>
                      <Th>Status</Th>
                      <Th>Change Note</Th>
                      <Th>Created By</Th>
                      <Th>Created At</Th>
                      {canWrite && <Th screenReaderText="Actions" />}
                    </Tr>
                  </Thead>
                  <Tbody>
                    {[...versions].sort((a, b) => b.version - a.version).map((v) => (
                      <Tr key={v.id}>
                        <Td dataLabel="Version">v{v.version}</Td>
                        <Td dataLabel="Status">
                          <StatusBadge status={v.is_active ? 'Active' : 'Inactive'} />
                        </Td>
                        <Td dataLabel="Change Note">{v.change_note || '-'}</Td>
                        <Td dataLabel="Created By">{v.created_by}</Td>
                        <Td dataLabel="Created At">
                          {new Date(v.created_at).toLocaleString()}
                        </Td>
                        {canWrite && (
                          <Td>
                            {!v.is_active && (
                              <Button
                                variant="secondary"
                                size="sm"
                                onClick={() => handleActivateVersion(v.version)}
                                data-testid={`activate-version-${v.version}`}
                              >
                                Activate
                              </Button>
                            )}
                          </Td>
                        )}
                      </Tr>
                    ))}
                  </Tbody>
                </Table>
              </>
            )}
          </TabContentBody>
        </Tab>
      </Tabs>

      {/* Create Version Modal */}
      <Modal
        isOpen={versionModalOpen}
        onClose={() => {
          setVersionModalOpen(false);
          setVersionError(null);
        }}
        title="Create New Version"
        variant={ModalVariant.medium}
        aria-label="Create New Version"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreateVersion}
            data-testid="create-version-submit"
          >
            Create Version
          </Button>,
          <Button
            key="cancel"
            variant="link"
            onClick={() => {
              setVersionModalOpen(false);
              setVersionError(null);
            }}
          >
            Cancel
          </Button>,
        ]}
      >
        {versionError && (
          <Alert variant="danger" title="Error" isInline style={{ marginBottom: '1rem' }}>
            {versionError}
          </Alert>
        )}
        <Form>
          <JsonEditor
            value={newConfigJson}
            onChange={setNewConfigJson}
            label="Configuration"
          />
          <FormGroup label="Change Note" fieldId="version-change-note">
            <TextInput
              id="version-change-note"
              value={newChangeNote}
              onChange={(_event, val) => setNewChangeNote(val)}
              placeholder="e.g., Lowered temperature for deterministic output"
            />
          </FormGroup>
        </Form>
      </Modal>
    </div>
  );
}
