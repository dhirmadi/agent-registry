import { useEffect, useState, useCallback } from 'react';
import type { FormEvent } from 'react';
import {
  Alert,
  Button,
  Card,
  CardBody,
  CardTitle,
  ClipboardCopy,
  Form,
  FormGroup,
  HelperText,
  HelperTextItem,
  Modal,
  Spinner,
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
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { useToast } from '../components/ToastNotifications';
import type { APIKey, APIKeyCreateResponse } from '../types';

const PASSWORD_RULES = [
  { test: (p: string) => p.length >= 12, label: 'At least 12 characters' },
  { test: (p: string) => /[A-Z]/.test(p), label: 'At least 1 uppercase letter' },
  { test: (p: string) => /[a-z]/.test(p), label: 'At least 1 lowercase letter' },
  { test: (p: string) => /\d/.test(p), label: 'At least 1 digit' },
  { test: (p: string) => /[^A-Za-z0-9]/.test(p), label: 'At least 1 special character' },
];

function validatePassword(password: string): string[] {
  return PASSWORD_RULES.filter((r) => !r.test(password)).map((r) => r.label);
}

export function MyAccountPage() {
  const { user } = useAuth();
  const { addToast } = useToast();

  // Password change form state
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordError, setPasswordError] = useState('');
  const [isChangingPassword, setIsChangingPassword] = useState(false);

  // API keys state
  const [apiKeys, setApiKeys] = useState<APIKey[]>([]);
  const [keysLoading, setKeysLoading] = useState(true);
  const [keysError, setKeysError] = useState<string | null>(null);

  // Create key modal
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false);
  const [newKeyName, setNewKeyName] = useState('');
  const [newKeyScopes, setNewKeyScopes] = useState('');
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [isCreatingKey, setIsCreatingKey] = useState(false);

  // Google linking state
  const [isUnlinking, setIsUnlinking] = useState(false);

  const validationErrors = newPassword ? validatePassword(newPassword) : [];
  const passwordsMatch = newPassword === confirmPassword;
  const canSubmitPassword =
    currentPassword &&
    newPassword &&
    confirmPassword &&
    validationErrors.length === 0 &&
    passwordsMatch &&
    !isChangingPassword;

  const fetchApiKeys = useCallback(async () => {
    setKeysLoading(true);
    setKeysError(null);
    try {
      const data = await api.get<{ items: APIKey[]; total: number }>('/api/v1/api-keys?mine=true');
      setApiKeys(data.items);
    } catch (err) {
      setKeysError(err instanceof Error ? err.message : 'Failed to load API keys');
    } finally {
      setKeysLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchApiKeys();
  }, [fetchApiKeys]);

  async function handleChangePassword(e: FormEvent) {
    e.preventDefault();
    if (!canSubmitPassword) return;

    setPasswordError('');
    setIsChangingPassword(true);
    try {
      await api.post('/auth/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
      });
      addToast('success', 'Password changed successfully');
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err) {
      setPasswordError(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setIsChangingPassword(false);
    }
  }

  async function handleUnlinkGoogle() {
    setIsUnlinking(true);
    try {
      await api.post('/auth/unlink-google');
      addToast('success', 'Google account unlinked');
    } catch (err) {
      addToast('danger', 'Failed to unlink Google', err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setIsUnlinking(false);
    }
  }

  function handleLinkGoogle() {
    window.location.href = '/auth/google/start';
  }

  async function handleCreateKey() {
    if (!newKeyName.trim()) return;
    setIsCreatingKey(true);
    try {
      const scopes = newKeyScopes
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
      const result = await api.post<APIKeyCreateResponse>('/api/v1/api-keys', {
        name: newKeyName,
        scopes: scopes.length > 0 ? scopes : ['read'],
      });
      setCreatedKey(result.raw_key);
      addToast('success', 'API key created');
      await fetchApiKeys();
    } catch (err) {
      addToast('danger', 'Failed to create key', err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setIsCreatingKey(false);
    }
  }

  async function handleRevokeKey(keyId: string) {
    try {
      await api.delete(`/api/v1/api-keys/${keyId}`);
      addToast('success', 'API key revoked');
      await fetchApiKeys();
    } catch (err) {
      addToast('danger', 'Failed to revoke key', err instanceof Error ? err.message : 'Unknown error');
    }
  }

  function closeCreateModal() {
    setIsCreateModalOpen(false);
    setNewKeyName('');
    setNewKeyScopes('');
    setCreatedKey(null);
  }

  const showPasswordSection = user?.auth_method !== 'google';
  const isGoogleLinked = user?.auth_method === 'google' || user?.auth_method === 'both';

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        My Account
      </Title>

      <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
        {/* Account Info */}
        <Card>
          <CardTitle>Account Information</CardTitle>
          <CardBody>
            <div>
              <strong>Name:</strong> {user?.display_name}
            </div>
            <div>
              <strong>Email:</strong> {user?.email}
            </div>
            <div>
              <strong>Role:</strong> {user?.role}
            </div>
            <div>
              <strong>Auth Method:</strong> {user?.auth_method}
            </div>
          </CardBody>
        </Card>

        {/* Password Change */}
        {showPasswordSection && (
          <Card data-testid="password-section">
            <CardTitle>Change Password</CardTitle>
            <CardBody>
              {passwordError && (
                <Alert variant="danger" title={passwordError} isInline style={{ marginBottom: '1rem' }} />
              )}

              <Form onSubmit={handleChangePassword}>
                <FormGroup label="Current Password" isRequired fieldId="current-password">
                  <TextInput
                    id="current-password"
                    type="password"
                    value={currentPassword}
                    onChange={(_e, val) => setCurrentPassword(val)}
                    isRequired
                  />
                </FormGroup>

                <FormGroup label="New Password" isRequired fieldId="new-password">
                  <TextInput
                    id="new-password"
                    type="password"
                    value={newPassword}
                    onChange={(_e, val) => setNewPassword(val)}
                    isRequired
                    validated={newPassword && validationErrors.length > 0 ? 'error' : 'default'}
                  />
                  {newPassword && validationErrors.length > 0 && (
                    <HelperText>
                      {validationErrors.map((msg) => (
                        <HelperTextItem key={msg} variant="error">
                          {msg}
                        </HelperTextItem>
                      ))}
                    </HelperText>
                  )}
                </FormGroup>

                <FormGroup label="Confirm New Password" isRequired fieldId="confirm-password">
                  <TextInput
                    id="confirm-password"
                    type="password"
                    value={confirmPassword}
                    onChange={(_e, val) => setConfirmPassword(val)}
                    isRequired
                    validated={confirmPassword && !passwordsMatch ? 'error' : 'default'}
                  />
                  {confirmPassword && !passwordsMatch && (
                    <HelperText>
                      <HelperTextItem variant="error">Passwords do not match</HelperTextItem>
                    </HelperText>
                  )}
                </FormGroup>

                <Button
                  type="submit"
                  variant="primary"
                  isDisabled={!canSubmitPassword}
                  isLoading={isChangingPassword}
                >
                  Change Password
                </Button>
              </Form>
            </CardBody>
          </Card>
        )}

        {/* Google Account */}
        <Card data-testid="google-section">
          <CardTitle>Google Account</CardTitle>
          <CardBody>
            {isGoogleLinked ? (
              <div>
                <p style={{ marginBottom: '1rem' }}>Your account is linked to Google.</p>
                <Button
                  variant="danger"
                  onClick={handleUnlinkGoogle}
                  isLoading={isUnlinking}
                  data-testid="unlink-google"
                >
                  Unlink Google Account
                </Button>
              </div>
            ) : (
              <div>
                <p style={{ marginBottom: '1rem' }}>Your account is not linked to Google.</p>
                <Button
                  variant="primary"
                  onClick={handleLinkGoogle}
                  data-testid="link-google"
                >
                  Link Google Account
                </Button>
              </div>
            )}
          </CardBody>
        </Card>

        {/* API Keys */}
        <Card data-testid="api-keys-section">
          <CardTitle>My API Keys</CardTitle>
          <CardBody>
            <div style={{ marginBottom: '1rem' }}>
              <Button
                variant="primary"
                onClick={() => setIsCreateModalOpen(true)}
                data-testid="create-api-key"
              >
                Create API Key
              </Button>
            </div>

            {keysLoading ? (
              <div data-testid="api-keys-loading" style={{ textAlign: 'center', padding: '2rem' }}>
                <Spinner aria-label="Loading API keys" />
              </div>
            ) : keysError ? (
              <Alert variant="danger" title={keysError} isInline />
            ) : apiKeys.length === 0 ? (
              <Alert variant="info" title="No API keys" isInline isPlain />
            ) : (
              <Table aria-label="My API keys" data-testid="api-keys-table">
                <Thead>
                  <Tr>
                    <Th>Name</Th>
                    <Th>Prefix</Th>
                    <Th>Scopes</Th>
                    <Th>Created</Th>
                    <Th>Last Used</Th>
                    <Th screenReaderText="Actions" />
                  </Tr>
                </Thead>
                <Tbody>
                  {apiKeys.map((key) => (
                    <Tr key={key.id}>
                      <Td dataLabel="Name">{key.name}</Td>
                      <Td dataLabel="Prefix">{key.key_prefix}</Td>
                      <Td dataLabel="Scopes">{key.scopes.join(', ')}</Td>
                      <Td dataLabel="Created">{new Date(key.created_at).toLocaleDateString()}</Td>
                      <Td dataLabel="Last Used">
                        {key.last_used_at ? new Date(key.last_used_at).toLocaleDateString() : 'Never'}
                      </Td>
                      <Td>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => handleRevokeKey(key.id)}
                          data-testid={`revoke-key-${key.id}`}
                        >
                          Revoke
                        </Button>
                      </Td>
                    </Tr>
                  ))}
                </Tbody>
              </Table>
            )}
          </CardBody>
        </Card>
      </div>

      {/* Create API Key Modal */}
      <Modal
        isOpen={isCreateModalOpen}
        onClose={closeCreateModal}
        title={createdKey ? 'API Key Created' : 'Create API Key'}
        variant="small"
        aria-label="Create API Key"
        actions={
          createdKey
            ? [
                <Button key="done" variant="primary" onClick={closeCreateModal}>
                  Done
                </Button>,
              ]
            : [
                <Button
                  key="submit"
                  variant="primary"
                  onClick={handleCreateKey}
                  isDisabled={!newKeyName.trim() || isCreatingKey}
                  isLoading={isCreatingKey}
                  data-testid="submit-create-key"
                >
                  Create
                </Button>,
                <Button key="cancel" variant="link" onClick={closeCreateModal}>
                  Cancel
                </Button>,
              ]
        }
      >
        {createdKey ? (
          <div>
            <Alert
              variant="warning"
              title="Copy your key now. It will not be shown again."
              isInline
              style={{ marginBottom: '1rem' }}
            />
            <ClipboardCopy isReadOnly data-testid="raw-key-display">
              {createdKey}
            </ClipboardCopy>
          </div>
        ) : (
          <Form>
            <FormGroup label="Key Name" isRequired fieldId="key-name">
              <TextInput
                id="key-name"
                value={newKeyName}
                onChange={(_e, val) => setNewKeyName(val)}
                isRequired
                data-testid="key-name-input"
              />
            </FormGroup>
            <FormGroup label="Scopes (comma-separated)" fieldId="key-scopes">
              <TextInput
                id="key-scopes"
                value={newKeyScopes}
                onChange={(_e, val) => setNewKeyScopes(val)}
                placeholder="read, write"
                data-testid="key-scopes-input"
              />
            </FormGroup>
          </Form>
        )}
      </Modal>
    </div>
  );
}
