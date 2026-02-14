import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  ClipboardCopy,
  Form,
  FormGroup,
  Label,
  Modal,
  ModalVariant,
  Spinner,
  TextInput,
  Title,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import type { Webhook } from '../types';

interface WebhooksListResponse {
  items: Webhook[];
  total: number;
}

interface WebhookCreateResponse {
  webhook: Webhook;
  secret: string;
}

const ALL_EVENTS = [
  'agent.created',
  'agent.updated',
  'agent.deleted',
  'mcp.created',
  'mcp.updated',
  'mcp.deleted',
  'prompt.created',
  'prompt.activated',
  'config.updated',
  'trigger.fired',
];

export function WebhooksPage() {
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [webhooks, setWebhooks] = useState<Webhook[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);
  const [createUrl, setCreateUrl] = useState('');
  const [createEvents, setCreateEvents] = useState<string[]>([]);
  const [createError, setCreateError] = useState<string | null>(null);

  const [secretModalOpen, setSecretModalOpen] = useState(false);
  const [createdSecret, setCreatedSecret] = useState('');

  const [confirmDelete, setConfirmDelete] = useState<string | null>(null);

  const fetchWebhooks = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<WebhooksListResponse>('/api/v1/webhooks');
      setWebhooks(data.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load webhooks');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchWebhooks();
  }, [fetchWebhooks]);

  async function handleCreate() {
    setCreateError(null);
    try {
      const data = await api.post<WebhookCreateResponse>('/api/v1/webhooks', {
        url: createUrl,
        events: createEvents,
      });
      setCreateOpen(false);
      setCreateUrl('');
      setCreateEvents([]);
      setCreatedSecret(data.secret);
      setSecretModalOpen(true);
      await fetchWebhooks();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create webhook');
    }
  }

  async function handleDelete(webhookId: string) {
    try {
      await api.delete(`/api/v1/webhooks/${webhookId}`);
      await fetchWebhooks();
    } catch {
      // Error handled by refetch
    }
    setConfirmDelete(null);
  }

  async function handleToggleActive(webhook: Webhook) {
    try {
      await api.patch(`/api/v1/webhooks/${webhook.id}`, {
        is_active: !webhook.is_active,
      });
      await fetchWebhooks();
    } catch {
      // Error handled by refetch
    }
  }

  function toggleEvent(event: string) {
    setCreateEvents((prev) =>
      prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event],
    );
  }

  if (loading) {
    return (
      <div data-testid="webhooks-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading webhooks" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading webhooks" data-testid="webhooks-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Webhooks
      </Title>

      <Toolbar>
        <ToolbarContent>
          {canWrite && (
            <ToolbarItem>
              <Button variant="primary" onClick={() => setCreateOpen(true)}>
                Create Webhook
              </Button>
            </ToolbarItem>
          )}
        </ToolbarContent>
      </Toolbar>

      {webhooks.length === 0 ? (
        <Alert variant="info" title="No webhooks configured" isInline isPlain />
      ) : (
        <Table aria-label="Webhooks table" data-testid="webhooks-table">
          <Thead>
            <Tr>
              <Th>URL</Th>
              <Th>Events</Th>
              <Th>Status</Th>
              <Th>Created</Th>
              {canWrite && <Th>Actions</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {webhooks.map((webhook) => (
              <Tr key={webhook.id} data-testid={`webhook-row-${webhook.id}`}>
                <Td dataLabel="URL">{webhook.url}</Td>
                <Td dataLabel="Events">
                  {webhook.events.map((e) => (
                    <Label key={e} style={{ marginRight: '0.25rem', marginBottom: '0.25rem' }}>
                      {e}
                    </Label>
                  ))}
                </Td>
                <Td dataLabel="Status">
                  <StatusBadge status={webhook.is_active ? 'Active' : 'Inactive'} />
                </Td>
                <Td dataLabel="Created">
                  {new Date(webhook.created_at).toLocaleDateString()}
                </Td>
                {canWrite && (
                  <Td>
                    <Button
                      variant="secondary"
                      size="sm"
                      onClick={() => handleToggleActive(webhook)}
                      style={{ marginRight: '0.5rem' }}
                    >
                      {webhook.is_active ? 'Deactivate' : 'Activate'}
                    </Button>
                    <Button
                      variant="danger"
                      size="sm"
                      data-testid={`delete-webhook-${webhook.id}`}
                      onClick={() => setConfirmDelete(webhook.id)}
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

      <ConfirmDialog
        isOpen={confirmDelete !== null}
        title="Delete Webhook"
        message="Are you sure you want to delete this webhook? This action cannot be undone."
        confirmText="Delete"
        variant="danger"
        onConfirm={() => confirmDelete && handleDelete(confirmDelete)}
        onCancel={() => setConfirmDelete(null)}
      />

      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Create Webhook"
        variant={ModalVariant.medium}
        aria-label="Create Webhook"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreate}
            isDisabled={!createUrl || createEvents.length === 0}
            data-testid="create-webhook-submit"
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
          <FormGroup label="URL" isRequired fieldId="webhook-url">
            <TextInput
              id="webhook-url"
              value={createUrl}
              onChange={(_event, val) => setCreateUrl(val)}
              placeholder="https://example.com/webhook"
            />
          </FormGroup>
          <FormGroup label="Events" isRequired fieldId="webhook-events">
            {ALL_EVENTS.map((event) => (
              <Checkbox
                key={event}
                id={`event-${event}`}
                label={event}
                isChecked={createEvents.includes(event)}
                onChange={() => toggleEvent(event)}
                style={{ marginBottom: '0.25rem' }}
              />
            ))}
          </FormGroup>
        </Form>
      </Modal>

      <Modal
        isOpen={secretModalOpen}
        onClose={() => {
          setSecretModalOpen(false);
          setCreatedSecret('');
        }}
        title="Webhook Secret"
        variant={ModalVariant.medium}
        aria-label="Webhook Secret"
        data-testid="secret-modal"
        actions={[
          <Button
            key="close"
            variant="primary"
            onClick={() => {
              setSecretModalOpen(false);
              setCreatedSecret('');
            }}
          >
            Close
          </Button>,
        ]}
      >
        <Alert
          variant="warning"
          title="Save this secret now"
          isInline
          style={{ marginBottom: '1rem' }}
        >
          This secret will only be shown once. Store it securely.
        </Alert>
        <ClipboardCopy isReadOnly hoverTip="Copy" clickTip="Copied" data-testid="webhook-secret-copy">
          {createdSecret}
        </ClipboardCopy>
      </Modal>
    </div>
  );
}
