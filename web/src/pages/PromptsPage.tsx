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
  Spinner,
  TextArea,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { CubesIcon } from '@patternfly/react-icons';
import { Table, Tbody, Td, Th, Thead, Tr } from '@patternfly/react-table';
import { api } from '../api/client';
import type { Agent, Prompt } from '../types';

interface AgentsResponse {
  agents: Agent[];
  total: number;
}

interface PromptsResponse {
  prompts: Prompt[];
  total: number;
}

interface PromptFormData {
  system_prompt: string;
  template_vars: Record<string, string>;
  mode: Prompt['mode'];
}

const MODE_OPTIONS: Prompt['mode'][] = ['rag_readonly', 'toolcalling_safe', 'toolcalling_auto'];

export function PromptsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selectedAgentId, setSelectedAgentId] = useState<string>('');
  const [prompts, setPrompts] = useState<Prompt[]>([]);
  const [loading, setLoading] = useState(true);
  const [promptsLoading, setPromptsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [modalOpen, setModalOpen] = useState(false);
  const [formData, setFormData] = useState<PromptFormData>({
    system_prompt: '',
    template_vars: {},
    mode: 'toolcalling_safe',
  });

  useEffect(() => {
    let cancelled = false;

    async function fetchAgents() {
      try {
        const data = await api.get<AgentsResponse>('/api/v1/agents?active_only=false');
        if (!cancelled) {
          setAgents(data.agents ?? []);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Failed to load agents');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    fetchAgents();
    return () => { cancelled = true; };
  }, []);

  const fetchPrompts = useCallback(async (agentId: string) => {
    setPromptsLoading(true);
    setError(null);
    try {
      const data = await api.get<PromptsResponse>(
        `/api/v1/agents/${agentId}/prompts?limit=100`,
      );
      setPrompts(data.prompts ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load prompts');
      setPrompts([]);
    } finally {
      setPromptsLoading(false);
    }
  }, []);

  const handleAgentChange = useCallback(
    (_event: React.FormEvent<HTMLSelectElement>, value: string) => {
      setSelectedAgentId(value);
      if (value) {
        fetchPrompts(value);
      } else {
        setPrompts([]);
      }
    },
    [fetchPrompts],
  );

  const handleCreate = useCallback(async () => {
    if (!selectedAgentId) return;
    try {
      await api.post(`/api/v1/agents/${selectedAgentId}/prompts`, {
        system_prompt: formData.system_prompt,
        template_vars: formData.template_vars,
        mode: formData.mode,
      });
      setModalOpen(false);
      setFormData({ system_prompt: '', template_vars: {}, mode: 'toolcalling_safe' });
      fetchPrompts(selectedAgentId);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create prompt');
    }
  }, [selectedAgentId, formData, fetchPrompts]);

  const handleActivate = useCallback(
    async (promptId: string) => {
      if (!selectedAgentId) return;
      try {
        await api.post(
          `/api/v1/agents/${selectedAgentId}/prompts/${promptId}/activate`,
        );
        fetchPrompts(selectedAgentId);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to activate prompt');
      }
    },
    [selectedAgentId, fetchPrompts],
  );

  if (loading) {
    return (
      <div data-testid="prompts-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading prompts page" />
      </div>
    );
  }

  if (error && !selectedAgentId) {
    return (
      <Alert variant="danger" title="Error" data-testid="prompts-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Prompts
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <FormSelect
              data-testid="agent-selector"
              value={selectedAgentId}
              onChange={handleAgentChange}
              aria-label="Select an agent"
            >
              <FormSelectOption value="" label="-- Select an Agent --" isPlaceholder />
              {agents.map((agent) => (
                <FormSelectOption key={agent.id} value={agent.id} label={agent.name} />
              ))}
            </FormSelect>
          </ToolbarItem>
          {selectedAgentId && (
            <ToolbarItem align={{ default: 'alignRight' }}>
              <Button
                variant="primary"
                data-testid="create-prompt-btn"
                onClick={() => setModalOpen(true)}
              >
                Create New Prompt
              </Button>
            </ToolbarItem>
          )}
        </ToolbarContent>
      </Toolbar>

      {error && selectedAgentId && (
        <Alert
          variant="danger"
          title="Error"
          data-testid="prompts-error"
          style={{ marginBottom: '1rem' }}
        >
          {error}
        </Alert>
      )}

      {promptsLoading && (
        <div data-testid="prompts-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading prompts" />
        </div>
      )}

      {selectedAgentId && !promptsLoading && !error && prompts.length === 0 && (
        <EmptyState data-testid="prompts-empty">
          <EmptyStateHeader
            titleText="No prompts found"
            icon={<EmptyStateIcon icon={CubesIcon} />}
            headingLevel="h2"
          />
          <EmptyStateBody>
            This agent has no prompt versions yet. Create one to get started.
          </EmptyStateBody>
        </EmptyState>
      )}

      {selectedAgentId && !promptsLoading && prompts.length > 0 && (
        <Table aria-label="Prompts table" data-testid="prompts-table">
          <Thead>
            <Tr>
              <Th>Version</Th>
              <Th>Mode</Th>
              <Th>Active</Th>
              <Th>Created By</Th>
              <Th>Created At</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {prompts.map((prompt) => (
              <Tr key={prompt.id} data-testid={`prompt-row-${prompt.id}`}>
                <Td>{prompt.version}</Td>
                <Td>{prompt.mode}</Td>
                <Td>
                  {prompt.is_active ? (
                    <Label color="green">Active</Label>
                  ) : (
                    <Label color="grey">Inactive</Label>
                  )}
                </Td>
                <Td>{prompt.created_by}</Td>
                <Td>{new Date(prompt.created_at).toLocaleString()}</Td>
                <Td>
                  {!prompt.is_active && (
                    <Button
                      variant="secondary"
                      size="sm"
                      data-testid="activate-prompt-btn"
                      onClick={() => handleActivate(prompt.id)}
                    >
                      Activate
                    </Button>
                  )}
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <Modal
        variant={ModalVariant.medium}
        title="Create New Prompt"
        isOpen={modalOpen}
        onClose={() => setModalOpen(false)}
        data-testid="prompt-modal"
        actions={[
          <Button
            key="save"
            variant="primary"
            data-testid="prompt-modal-save"
            onClick={handleCreate}
          >
            Save
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setModalOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <FormGroup label="System Prompt" isRequired fieldId="system-prompt">
          <TextArea
            id="system-prompt"
            aria-label="System Prompt"
            value={formData.system_prompt}
            onChange={(_event, value) =>
              setFormData((prev) => ({ ...prev, system_prompt: value }))
            }
            rows={8}
          />
        </FormGroup>
        <FormGroup label="Mode" isRequired fieldId="prompt-mode" style={{ marginTop: '1rem' }}>
          <FormSelect
            id="prompt-mode"
            aria-label="Mode"
            value={formData.mode}
            onChange={(_event, value) =>
              setFormData((prev) => ({ ...prev, mode: value as Prompt['mode'] }))
            }
          >
            {MODE_OPTIONS.map((mode) => (
              <FormSelectOption key={mode} value={mode} label={mode} />
            ))}
          </FormSelect>
        </FormGroup>
      </Modal>
    </div>
  );
}
