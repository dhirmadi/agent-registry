import { useEffect, useState, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  Label,
  Spinner,
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
import type { Agent, AgentVersion, Prompt } from '../types';

interface VersionsResponse {
  versions: AgentVersion[];
  total: number;
}

interface PromptsResponse {
  prompts: Prompt[] | null;
  total: number;
}

export function AgentDetailPage() {
  const { agentId } = useParams<{ agentId: string }>();
  const navigate = useNavigate();
  const { user } = useAuth();
  const { addToast } = useToast();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [agent, setAgent] = useState<Agent | null>(null);
  const [versions, setVersions] = useState<AgentVersion[]>([]);
  const [prompts, setPrompts] = useState<Prompt[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState(0);
  const [conflictMessage, setConflictMessage] = useState<string | null>(null);

  // Editable form state
  const [editName, setEditName] = useState('');
  const [editDesc, setEditDesc] = useState('');
  const [editPrompts, setEditPrompts] = useState('');
  const [editSystemPrompt, setEditSystemPrompt] = useState('');

  const fetchData = useCallback(async () => {
    if (!agentId) return;
    setLoading(true);
    setError(null);
    try {
      const [agentData, versionsData, promptsData] = await Promise.all([
        api.get<Agent>(`/api/v1/agents/${agentId}`),
        api.get<VersionsResponse>(`/api/v1/agents/${agentId}/versions`),
        api.get<PromptsResponse>(`/api/v1/agents/${agentId}/prompts`),
      ]);
      setAgent(agentData);
      setVersions(versionsData.versions ?? []);
      setPrompts(promptsData.prompts ?? []);
      // Populate form state
      setEditName(agentData.name);
      setEditDesc(agentData.description);
      setEditPrompts(JSON.stringify(agentData.example_prompts, null, 2));
      setEditSystemPrompt(agentData.system_prompt);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load agent');
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  async function handleSaveGeneral() {
    if (!agent) return;
    setConflictMessage(null);
    try {
      let parsedPrompts: string[] = [];
      try {
        parsedPrompts = JSON.parse(editPrompts);
      } catch {
        parsedPrompts = editPrompts.split('\n').filter((l) => l.trim());
      }

      await api.put(
        `/api/v1/agents/${agent.id}`,
        {
          name: editName,
          description: editDesc,
          example_prompts: parsedPrompts,
          system_prompt: editSystemPrompt,
          tools: agent.tools,
          trust_overrides: agent.trust_overrides,
        },
        agent.updated_at,
      );
      addToast('success', 'Agent saved', `Saved as new version`);
      await fetchData();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        setConflictMessage(err.message);
      } else {
        addToast('danger', 'Save failed', err instanceof Error ? err.message : 'Unknown error');
      }
    }
  }

  async function handleSaveSystemPrompt() {
    if (!agent) return;
    setConflictMessage(null);
    try {
      await api.put(
        `/api/v1/agents/${agent.id}`,
        {
          name: agent.name,
          description: agent.description,
          example_prompts: agent.example_prompts,
          system_prompt: editSystemPrompt,
          tools: agent.tools,
          trust_overrides: agent.trust_overrides,
        },
        agent.updated_at,
      );
      addToast('success', 'System prompt saved');
      await fetchData();
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        setConflictMessage(err.message);
      } else {
        addToast('danger', 'Save failed', err instanceof Error ? err.message : 'Unknown error');
      }
    }
  }

  async function handleRollback(targetVersion: number) {
    if (!agent) return;
    try {
      await api.post(`/api/v1/agents/${agent.id}/rollback`, { target_version: targetVersion });
      addToast('success', 'Rollback complete', `Rolled back to version ${targetVersion}`);
      await fetchData();
    } catch (err) {
      addToast('danger', 'Rollback failed', err instanceof Error ? err.message : 'Unknown error');
    }
  }

  async function handleActivatePrompt(promptId: string) {
    if (!agent) return;
    try {
      await api.post(`/api/v1/agents/${agent.id}/prompts/${promptId}/activate`);
      addToast('success', 'Prompt activated');
      await fetchData();
    } catch (err) {
      addToast('danger', 'Activation failed', err instanceof Error ? err.message : 'Unknown error');
    }
  }

  if (loading) {
    return (
      <div data-testid="agent-detail-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading agent" />
      </div>
    );
  }

  if (error || !agent) {
    return (
      <Alert variant="danger" title="Error" data-testid="agent-detail-error">
        {error ?? 'Agent not found'}
      </Alert>
    );
  }

  const trustEntries = Object.entries(agent.trust_overrides);

  const timelineVersions: TimelineVersion[] = versions.map((v) => ({
    version: v.version,
    created_by: v.created_by,
    created_at: v.created_at,
    is_current: v.version === agent.version,
  }));

  return (
    <div>
      <div style={{ marginBottom: '1rem' }}>
        <Button variant="link" onClick={() => navigate('/agents')}>
          Back to Agents
        </Button>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.5rem' }}>
        <Title headingLevel="h1">{agent.name}</Title>
        <StatusBadge status={agent.is_active ? 'Active' : 'Inactive'} />
        <Label color="blue">v{agent.version}</Label>
        <Label>{agent.id}</Label>
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
        <Tab eventKey={0} title={<TabTitleText>General</TabTitleText>}>
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
              <FormGroup label="Description" fieldId="edit-desc">
                <TextArea
                  id="edit-desc"
                  value={editDesc}
                  onChange={(_event, val) => setEditDesc(val)}
                  rows={3}
                  isDisabled={!canWrite}
                />
              </FormGroup>
              <FormGroup label="Example Prompts" fieldId="edit-example-prompts">
                <TextArea
                  id="edit-example-prompts"
                  value={editPrompts}
                  onChange={(_event, val) => setEditPrompts(val)}
                  rows={4}
                  isDisabled={!canWrite}
                />
              </FormGroup>
              {canWrite && (
                <Button
                  variant="primary"
                  onClick={handleSaveGeneral}
                  data-testid="save-general"
                >
                  Save
                </Button>
              )}
            </Form>
          </TabContentBody>
        </Tab>

        <Tab eventKey={1} title={<TabTitleText>Tools</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            {agent.tools.length === 0 ? (
              <Alert variant="info" title="No tools configured" isInline isPlain />
            ) : (
              <Table aria-label="Agent tools">
                <Thead>
                  <Tr>
                    <Th>Name</Th>
                    <Th>Source</Th>
                    <Th>Server Label</Th>
                    <Th>Description</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {agent.tools.map((tool, i) => (
                    <Tr key={i}>
                      <Td dataLabel="Name">{tool.name}</Td>
                      <Td dataLabel="Source">{tool.source}</Td>
                      <Td dataLabel="Server Label">{tool.server_label || '-'}</Td>
                      <Td dataLabel="Description">{tool.description}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </TabContentBody>
        </Tab>

        <Tab eventKey={2} title={<TabTitleText>Trust Overrides</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            {trustEntries.length === 0 ? (
              <Alert variant="info" title="No trust overrides configured" isInline isPlain />
            ) : (
              <Table aria-label="Trust overrides">
                <Thead>
                  <Tr>
                    <Th>Tool Name</Th>
                    <Th>Tier</Th>
                  </Tr>
                </Thead>
                <Tbody>
                  {trustEntries.map(([tool, tier]) => (
                    <Tr key={tool}>
                      <Td dataLabel="Tool Name">{tool}</Td>
                      <Td dataLabel="Tier">{tier}</Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </TabContentBody>
        </Tab>

        <Tab eventKey={3} title={<TabTitleText>System Prompt</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            <TextArea
              id="edit-system-prompt"
              value={editSystemPrompt}
              onChange={(_event, val) => setEditSystemPrompt(val)}
              rows={20}
              isDisabled={!canWrite}
              data-testid="system-prompt-editor"
              style={{
                fontFamily: 'monospace',
                fontSize: '0.875rem',
              }}
            />
            {canWrite && (
              <Button
                variant="primary"
                onClick={handleSaveSystemPrompt}
                style={{ marginTop: '1rem' }}
                data-testid="save-system-prompt"
              >
                Save System Prompt
              </Button>
            )}
          </TabContentBody>
        </Tab>

        <Tab eventKey={4} title={<TabTitleText>Prompt Versions</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            {prompts.length === 0 ? (
              <Alert variant="info" title="No prompt versions" isInline isPlain />
            ) : (
              <Table aria-label="Prompt versions">
                <Thead>
                  <Tr>
                    <Th>Version</Th>
                    <Th>Mode</Th>
                    <Th>Status</Th>
                    <Th>Created By</Th>
                    <Th>Created At</Th>
                    {canWrite && <Th screenReaderText="Actions" />}
                  </Tr>
                </Thead>
                <Tbody>
                  {prompts.map((p) => (
                    <Tr key={p.id}>
                      <Td dataLabel="Version">{p.version}</Td>
                      <Td dataLabel="Mode">{p.mode}</Td>
                      <Td dataLabel="Status">
                        <StatusBadge status={p.is_active ? 'Active' : 'Inactive'} />
                      </Td>
                      <Td dataLabel="Created By">{p.created_by}</Td>
                      <Td dataLabel="Created At">
                        {new Date(p.created_at).toLocaleString()}
                      </Td>
                      {canWrite && (
                        <Td>
                          {!p.is_active && (
                            <Button
                              variant="secondary"
                              size="sm"
                              onClick={() => handleActivatePrompt(p.id)}
                              data-testid={`activate-prompt-${p.id}`}
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
            )}
          </TabContentBody>
        </Tab>

        <Tab eventKey={5} title={<TabTitleText>Version History</TabTitleText>}>
          <TabContentBody style={{ paddingTop: '1rem' }}>
            {versions.length === 0 ? (
              <Alert variant="info" title="No version history" isInline isPlain />
            ) : (
              <VersionTimeline
                versions={timelineVersions}
                onRollback={handleRollback}
                readOnly={!canWrite}
              />
            )}
          </TabContentBody>
        </Tab>
      </Tabs>
    </div>
  );
}
