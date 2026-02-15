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
import { useToast } from '../components/ToastNotifications';
import { StatusBadge } from '../components/StatusBadge';
import { ConfirmDialog } from '../components/ConfirmDialog';
import type { APIKey, APIKeyCreateResponse } from '../types';

interface APIKeysListResponse {
  keys: APIKey[];
  total: number;
}

const ALL_SCOPES = ['read', 'write', 'admin'];

export function APIKeysPage() {
  const { user } = useAuth();
  const { addToast } = useToast();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [keys, setKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState('');
  const [createScopes, setCreateScopes] = useState<string[]>([]);
  const [createExpiresAt, setCreateExpiresAt] = useState('');
  const [createError, setCreateError] = useState<string | null>(null);

  const [rawKeyModalOpen, setRawKeyModalOpen] = useState(false);
  const [createdRawKey, setCreatedRawKey] = useState('');

  const [confirmRevoke, setConfirmRevoke] = useState<string | null>(null);

  const fetchKeys = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<APIKeysListResponse>('/api/v1/api-keys');
      setKeys(data.keys ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load API keys');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchKeys();
  }, [fetchKeys]);

  async function handleCreate() {
    setCreateError(null);
    try {
      const body: Record<string, unknown> = {
        name: createName,
        scopes: createScopes,
      };
      if (createExpiresAt) {
        body.expires_at = createExpiresAt;
      }
      const data = await api.post<APIKeyCreateResponse>('/api/v1/api-keys', body);
      setCreateOpen(false);
      setCreateName('');
      setCreateScopes([]);
      setCreateExpiresAt('');
      setCreatedRawKey(data.key);
      setRawKeyModalOpen(true);
      await fetchKeys();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create API key');
    }
  }

  async function handleRevoke(keyId: string) {
    try {
      await api.delete(`/api/v1/api-keys/${keyId}`);
      await fetchKeys();
    } catch (err) {
      addToast('danger', 'Operation failed', err instanceof Error ? err.message : 'An unknown error occurred');
    }
    setConfirmRevoke(null);
  }

  function toggleScope(scope: string) {
    setCreateScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope],
    );
  }

  if (loading) {
    return (
      <div data-testid="apikeys-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading API keys" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading API keys" data-testid="apikeys-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        API Keys
      </Title>

      <Toolbar>
        <ToolbarContent>
          {canWrite && (
            <ToolbarItem align={{ default: 'alignRight' }}>
              <Button variant="primary" onClick={() => setCreateOpen(true)}>
                Create API Key
              </Button>
            </ToolbarItem>
          )}
        </ToolbarContent>
      </Toolbar>

      {keys.length === 0 ? (
        <Alert variant="info" title="No API keys found" isInline isPlain />
      ) : (
        <Table aria-label="API Keys table" data-testid="apikeys-table">
          <Thead>
            <Tr>
              <Th>Name</Th>
              <Th>Prefix</Th>
              <Th>Scopes</Th>
              <Th>Status</Th>
              <Th>Created</Th>
              <Th>Expires</Th>
              <Th>Last Used</Th>
              {canWrite && <Th>Actions</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {keys.map((k) => (
              <Tr key={k.id} data-testid={`apikey-row-${k.id}`}>
                <Td dataLabel="Name">{k.name}</Td>
                <Td dataLabel="Prefix">{k.key_prefix}</Td>
                <Td dataLabel="Scopes">
                  {(k.scopes ?? []).map((s) => (
                    <Label key={s} style={{ marginRight: '0.25rem' }}>
                      {s}
                    </Label>
                  ))}
                </Td>
                <Td dataLabel="Status">
                  <StatusBadge status={k.is_active !== false ? 'Active' : 'Inactive'} />
                </Td>
                <Td dataLabel="Created">
                  {new Date(k.created_at).toLocaleDateString()}
                </Td>
                <Td dataLabel="Expires">
                  {k.expires_at ? new Date(k.expires_at).toLocaleDateString() : 'Never'}
                </Td>
                <Td dataLabel="Last Used">
                  {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : 'Never'}
                </Td>
                {canWrite && (
                  <Td>
                    <Button
                      variant="danger"
                      size="sm"
                      data-testid={`revoke-key-${k.id}`}
                      onClick={() => setConfirmRevoke(k.id)}
                    >
                      Revoke
                    </Button>
                  </Td>
                )}
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <ConfirmDialog
        isOpen={confirmRevoke !== null}
        title="Revoke API Key"
        message="Are you sure you want to revoke this API key? This action cannot be undone."
        confirmText="Revoke"
        variant="danger"
        onConfirm={() => confirmRevoke && handleRevoke(confirmRevoke)}
        onCancel={() => setConfirmRevoke(null)}
      />

      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Create API Key"
        variant={ModalVariant.medium}
        aria-label="Create API Key"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreate}
            isDisabled={!createName || createScopes.length === 0}
            data-testid="create-apikey-submit"
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
          <FormGroup label="Name" isRequired fieldId="apikey-name">
            <TextInput
              id="apikey-name"
              value={createName}
              onChange={(_event, val) => setCreateName(val)}
              placeholder="e.g., CI Pipeline Key"
            />
          </FormGroup>
          <FormGroup label="Scopes" isRequired fieldId="apikey-scopes">
            {ALL_SCOPES.map((scope) => (
              <Checkbox
                key={scope}
                id={`scope-${scope}`}
                label={scope}
                isChecked={createScopes.includes(scope)}
                onChange={() => toggleScope(scope)}
                style={{ marginBottom: '0.25rem' }}
              />
            ))}
          </FormGroup>
          <FormGroup label="Expires At (optional)" fieldId="apikey-expires">
            <TextInput
              id="apikey-expires"
              type="date"
              value={createExpiresAt}
              onChange={(_event, val) => setCreateExpiresAt(val)}
            />
          </FormGroup>
        </Form>
      </Modal>

      <Modal
        isOpen={rawKeyModalOpen}
        onClose={() => {
          setRawKeyModalOpen(false);
          setCreatedRawKey('');
        }}
        title="API Key Created"
        variant={ModalVariant.medium}
        aria-label="API Key Created"
        data-testid="rawkey-modal"
        actions={[
          <Button
            key="close"
            variant="primary"
            onClick={() => {
              setRawKeyModalOpen(false);
              setCreatedRawKey('');
            }}
          >
            Close
          </Button>,
        ]}
      >
        <Alert
          variant="warning"
          title="Save this key now"
          isInline
          style={{ marginBottom: '1rem' }}
        >
          This API key will only be shown once. Store it securely.
        </Alert>
        <ClipboardCopy isReadOnly hoverTip="Copy" clickTip="Copied" data-testid="raw-key-copy">
          {createdRawKey}
        </ClipboardCopy>
      </Modal>
    </div>
  );
}
