import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Modal,
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
} from '@patternfly/react-table';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { JsonEditor } from '../components/JsonEditor';
import { StatusBadge } from '../components/StatusBadge';
import type { TriggerRule } from '../types';

interface TriggersResponse {
  items: TriggerRule[];
  total: number;
}

const EVENT_TYPES = [
  'message_received',
  'schedule',
  'webhook_event',
  'file_changed',
  'manual',
];

export function TriggersPage() {
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [triggers, setTriggers] = useState<TriggerRule[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Create modal state
  const [createOpen, setCreateOpen] = useState(false);
  const [formName, setFormName] = useState('');
  const [formEventType, setFormEventType] = useState(EVENT_TYPES[0]);
  const [formAgentId, setFormAgentId] = useState('');
  const [formCondition, setFormCondition] = useState('{}');
  const [formPromptTemplate, setFormPromptTemplate] = useState('');
  const [formEnabled, setFormEnabled] = useState(true);
  const [formRateLimit, setFormRateLimit] = useState('10');
  const [formSchedule, setFormSchedule] = useState('');

  // Delete
  const [deleteTarget, setDeleteTarget] = useState<TriggerRule | null>(null);

  const fetchTriggers = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<TriggersResponse>('/api/v1/triggers');
      setTriggers(data.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load triggers');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchTriggers();
  }, [fetchTriggers]);

  async function handleToggleEnabled(trigger: TriggerRule) {
    try {
      await api.patch(
        `/api/v1/triggers/${trigger.id}`,
        { enabled: !trigger.enabled },
        trigger.updated_at,
      );
      await fetchTriggers();
    } catch {
      // Error handled by refetch
    }
  }

  async function handleCreate() {
    try {
      let condition: Record<string, unknown> = {};
      try {
        condition = JSON.parse(formCondition);
      } catch {
        // Use empty object if invalid
      }

      await api.post('/api/v1/triggers', {
        name: formName,
        event_type: formEventType,
        agent_id: formAgentId,
        condition,
        prompt_template: formPromptTemplate,
        enabled: formEnabled,
        rate_limit_per_hour: Number(formRateLimit),
        schedule: formSchedule,
      });

      setCreateOpen(false);
      resetForm();
      await fetchTriggers();
    } catch {
      // Error handled by refetch
    }
  }

  async function handleDelete() {
    if (!deleteTarget) return;
    try {
      await api.delete(`/api/v1/triggers/${deleteTarget.id}`);
      await fetchTriggers();
    } catch {
      // Error handled by refetch
    }
    setDeleteTarget(null);
  }

  function resetForm() {
    setFormName('');
    setFormEventType(EVENT_TYPES[0]);
    setFormAgentId('');
    setFormCondition('{}');
    setFormPromptTemplate('');
    setFormEnabled(true);
    setFormRateLimit('10');
    setFormSchedule('');
  }

  if (loading) {
    return (
      <div data-testid="triggers-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading triggers" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading triggers" data-testid="triggers-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Trigger Rules
      </Title>

      {canWrite && (
        <Toolbar>
          <ToolbarContent>
            <ToolbarItem>
              <Button variant="primary" onClick={() => setCreateOpen(true)} data-testid="create-trigger-btn">
                Create Trigger
              </Button>
            </ToolbarItem>
          </ToolbarContent>
        </Toolbar>
      )}

      {triggers.length === 0 ? (
        <Alert variant="info" title="No trigger rules configured" isInline isPlain />
      ) : (
        <Table aria-label="Trigger rules table" data-testid="triggers-table">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Event Type</Th>
              <Th>Agent ID</Th>
              <Th>Status</Th>
              <Th>Schedule</Th>
              <Th>Rate Limit/hr</Th>
              {canWrite && <Th>Actions</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {triggers.map((trigger) => (
              <Tr key={trigger.id} data-testid={`trigger-row-${trigger.id}`}>
                <Td dataLabel="Name">{trigger.name}</Td>
                <Td dataLabel="Event Type">{trigger.event_type}</Td>
                <Td dataLabel="Agent ID">{trigger.agent_id}</Td>
                <Td dataLabel="Status">
                  <StatusBadge status={trigger.enabled ? 'Enabled' : 'Disabled'} />
                </Td>
                <Td dataLabel="Schedule">{trigger.schedule || '-'}</Td>
                <Td dataLabel="Rate Limit/hr">{trigger.rate_limit_per_hour}</Td>
                {canWrite && (
                  <Td>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleToggleEnabled(trigger)}
                      data-testid={`toggle-trigger-${trigger.id}`}
                      style={{ marginRight: '0.5rem' }}
                    >
                      {trigger.enabled ? 'Disable' : 'Enable'}
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setDeleteTarget(trigger)}
                      data-testid={`delete-trigger-${trigger.id}`}
                    >
                      Delete
                    </Button>
                  </Td>
                )}
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      {/* Create Modal */}
      <Modal
        isOpen={createOpen}
        onClose={() => { setCreateOpen(false); resetForm(); }}
        title="Create Trigger Rule"
        variant="large"
        aria-label="Create Trigger Rule"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreate}
            isDisabled={!formName || !formAgentId}
            data-testid="submit-trigger-btn"
          >
            Create
          </Button>,
          <Button key="cancel" variant="link" onClick={() => { setCreateOpen(false); resetForm(); }}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <FormGroup label="Name" isRequired fieldId="trigger-name">
            <TextInput
              id="trigger-name"
              value={formName}
              onChange={(_event, val) => setFormName(val)}
              placeholder="e.g., On new message"
            />
          </FormGroup>
          <FormGroup label="Event Type" isRequired fieldId="trigger-event-type">
            <FormSelect
              id="trigger-event-type"
              value={formEventType}
              onChange={(_event, val) => setFormEventType(val)}
              aria-label="Event Type"
            >
              {EVENT_TYPES.map((et) => (
                <FormSelectOption key={et} value={et} label={et} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Agent ID" isRequired fieldId="trigger-agent-id">
            <TextInput
              id="trigger-agent-id"
              value={formAgentId}
              onChange={(_event, val) => setFormAgentId(val)}
              placeholder="e.g., pmo"
            />
          </FormGroup>
          <JsonEditor
            label="Condition (JSON)"
            value={formCondition}
            onChange={setFormCondition}
          />
          <FormGroup label="Prompt Template" fieldId="trigger-prompt-template">
            <TextArea
              id="trigger-prompt-template"
              value={formPromptTemplate}
              onChange={(_event, val) => setFormPromptTemplate(val)}
              placeholder="Optional prompt template"
              rows={3}
            />
          </FormGroup>
          <FormGroup label="Enabled" fieldId="trigger-enabled">
            <Switch
              id="trigger-enabled"
              isChecked={formEnabled}
              onChange={(_event, checked) => setFormEnabled(checked)}
              aria-label="Enabled"
            />
          </FormGroup>
          <FormGroup label="Rate Limit (per hour)" fieldId="trigger-rate-limit">
            <TextInput
              id="trigger-rate-limit"
              type="number"
              value={formRateLimit}
              onChange={(_event, val) => setFormRateLimit(val)}
            />
          </FormGroup>
          <FormGroup label="Schedule (cron)" fieldId="trigger-schedule">
            <TextInput
              id="trigger-schedule"
              value={formSchedule}
              onChange={(_event, val) => setFormSchedule(val)}
              placeholder="e.g., 0 */5 * * *"
            />
          </FormGroup>
        </Form>
      </Modal>

      {/* Delete Confirmation */}
      <ConfirmDialog
        isOpen={deleteTarget !== null}
        title="Delete Trigger Rule"
        message={`Are you sure you want to delete trigger "${deleteTarget?.name}"?`}
        confirmText="Delete"
        variant="danger"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  );
}
