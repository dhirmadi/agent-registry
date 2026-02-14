import { useEffect, useState, useCallback } from 'react';
import {
  Alert,
  Button,
  Form,
  FormGroup,
  FormSelect,
  FormSelectOption,
  Modal,
  ModalVariant,
  Spinner,
  Switch,
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
import type { UserAdmin } from '../types';

interface UsersListResponse {
  items: UserAdmin[];
  total: number;
}

const ROLE_OPTIONS: UserAdmin['role'][] = ['admin', 'editor', 'viewer'];

export function UsersPage() {
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';

  const [users, setUsers] = useState<UserAdmin[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [createOpen, setCreateOpen] = useState(false);
  const [createUsername, setCreateUsername] = useState('');
  const [createEmail, setCreateEmail] = useState('');
  const [createDisplayName, setCreateDisplayName] = useState('');
  const [createRole, setCreateRole] = useState<UserAdmin['role']>('viewer');
  const [createPassword, setCreatePassword] = useState('');
  const [createError, setCreateError] = useState<string | null>(null);

  const [editOpen, setEditOpen] = useState(false);
  const [editUser, setEditUser] = useState<UserAdmin | null>(null);
  const [editRole, setEditRole] = useState<UserAdmin['role']>('viewer');
  const [editActive, setEditActive] = useState(true);
  const [editError, setEditError] = useState<string | null>(null);

  const [confirmResetId, setConfirmResetId] = useState<string | null>(null);

  const fetchUsers = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<UsersListResponse>('/api/v1/users');
      setUsers(data.items ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load users');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  async function handleCreate() {
    setCreateError(null);
    try {
      await api.post('/api/v1/users', {
        username: createUsername,
        email: createEmail,
        display_name: createDisplayName,
        role: createRole,
        temporary_password: createPassword,
      });
      setCreateOpen(false);
      setCreateUsername('');
      setCreateEmail('');
      setCreateDisplayName('');
      setCreateRole('viewer');
      setCreatePassword('');
      await fetchUsers();
    } catch (err) {
      setCreateError(err instanceof Error ? err.message : 'Failed to create user');
    }
  }

  function openEditModal(u: UserAdmin) {
    setEditUser(u);
    setEditRole(u.role);
    setEditActive(u.is_active);
    setEditError(null);
    setEditOpen(true);
  }

  async function handleEdit() {
    if (!editUser) return;
    setEditError(null);
    try {
      await api.patch(
        `/api/v1/users/${editUser.id}`,
        { role: editRole, is_active: editActive },
        editUser.updated_at,
      );
      setEditOpen(false);
      setEditUser(null);
      await fetchUsers();
    } catch (err) {
      setEditError(err instanceof Error ? err.message : 'Failed to update user');
    }
  }

  async function handleForcePasswordReset(userId: string) {
    try {
      await api.post(`/api/v1/users/${userId}/force-password-reset`);
      setConfirmResetId(null);
      await fetchUsers();
    } catch {
      // Error handled by refetch
    }
  }

  if (!isAdmin) {
    return (
      <Alert variant="danger" title="Access Denied" data-testid="users-access-denied">
        You must be an administrator to view this page.
      </Alert>
    );
  }

  if (loading) {
    return (
      <div data-testid="users-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading users" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading users" data-testid="users-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Users
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <Button variant="primary" onClick={() => setCreateOpen(true)}>
              Create User
            </Button>
          </ToolbarItem>
        </ToolbarContent>
      </Toolbar>

      {users.length === 0 ? (
        <Alert variant="info" title="No users found" isInline isPlain />
      ) : (
        <Table aria-label="Users table" data-testid="users-table">
          <Thead>
            <Tr>
              <Th>Username</Th>
              <Th>Email</Th>
              <Th>Display Name</Th>
              <Th>Role</Th>
              <Th>Auth Method</Th>
              <Th>Status</Th>
              <Th>Last Login</Th>
              <Th>Actions</Th>
            </Tr>
          </Thead>
          <Tbody>
            {users.map((u) => (
              <Tr key={u.id} data-testid={`user-row-${u.id}`}>
                <Td dataLabel="Username">{u.username}</Td>
                <Td dataLabel="Email">{u.email}</Td>
                <Td dataLabel="Display Name">{u.display_name}</Td>
                <Td dataLabel="Role">{u.role}</Td>
                <Td dataLabel="Auth Method">{u.auth_method}</Td>
                <Td dataLabel="Status">
                  <StatusBadge status={u.is_active ? 'Active' : 'Inactive'} />
                </Td>
                <Td dataLabel="Last Login">
                  {u.last_login_at ? new Date(u.last_login_at).toLocaleDateString() : 'Never'}
                </Td>
                <Td>
                  <Button
                    variant="secondary"
                    size="sm"
                    data-testid={`edit-user-${u.id}`}
                    onClick={() => openEditModal(u)}
                    style={{ marginRight: '0.5rem' }}
                  >
                    Edit
                  </Button>
                  <Button
                    variant="warning"
                    size="sm"
                    data-testid={`reset-password-${u.id}`}
                    onClick={() => setConfirmResetId(u.id)}
                  >
                    Reset Password
                  </Button>
                </Td>
              </Tr>
            ))}
          </Tbody>
        </Table>
      )}

      <ConfirmDialog
        isOpen={confirmResetId !== null}
        title="Force Password Reset"
        message="Are you sure you want to force a password reset for this user?"
        confirmText="Reset"
        variant="warning"
        onConfirm={() => confirmResetId && handleForcePasswordReset(confirmResetId)}
        onCancel={() => setConfirmResetId(null)}
      />

      <Modal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        title="Create User"
        variant={ModalVariant.medium}
        aria-label="Create User"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleCreate}
            isDisabled={!createUsername || !createEmail || !createPassword}
            data-testid="create-user-submit"
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
          <FormGroup label="Username" isRequired fieldId="user-username">
            <TextInput
              id="user-username"
              value={createUsername}
              onChange={(_event, val) => setCreateUsername(val)}
              placeholder="e.g., jdoe"
            />
          </FormGroup>
          <FormGroup label="Email" isRequired fieldId="user-email">
            <TextInput
              id="user-email"
              type="email"
              value={createEmail}
              onChange={(_event, val) => setCreateEmail(val)}
              placeholder="e.g., jdoe@example.com"
            />
          </FormGroup>
          <FormGroup label="Display Name" fieldId="user-display-name">
            <TextInput
              id="user-display-name"
              value={createDisplayName}
              onChange={(_event, val) => setCreateDisplayName(val)}
              placeholder="e.g., John Doe"
            />
          </FormGroup>
          <FormGroup label="Role" isRequired fieldId="user-role">
            <FormSelect
              id="user-role"
              value={createRole}
              onChange={(_event, val) => setCreateRole(val as UserAdmin['role'])}
            >
              {ROLE_OPTIONS.map((r) => (
                <FormSelectOption key={r} value={r} label={r} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Temporary Password" isRequired fieldId="user-password">
            <TextInput
              id="user-password"
              type="password"
              value={createPassword}
              onChange={(_event, val) => setCreatePassword(val)}
            />
          </FormGroup>
        </Form>
      </Modal>

      <Modal
        isOpen={editOpen}
        onClose={() => {
          setEditOpen(false);
          setEditUser(null);
        }}
        title={`Edit User: ${editUser?.username ?? ''}`}
        variant={ModalVariant.medium}
        aria-label="Edit User"
        data-testid="edit-user-modal"
        actions={[
          <Button
            key="submit"
            variant="primary"
            onClick={handleEdit}
            data-testid="edit-user-submit"
          >
            Save
          </Button>,
          <Button
            key="cancel"
            variant="link"
            onClick={() => {
              setEditOpen(false);
              setEditUser(null);
            }}
          >
            Cancel
          </Button>,
        ]}
      >
        {editError && (
          <Alert variant="danger" title="Error" isInline style={{ marginBottom: '1rem' }}>
            {editError}
          </Alert>
        )}
        <Form>
          <FormGroup label="Role" isRequired fieldId="edit-user-role">
            <FormSelect
              id="edit-user-role"
              value={editRole}
              onChange={(_event, val) => setEditRole(val as UserAdmin['role'])}
            >
              {ROLE_OPTIONS.map((r) => (
                <FormSelectOption key={r} value={r} label={r} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Active" fieldId="edit-user-active">
            <Switch
              id="edit-user-active"
              isChecked={editActive}
              onChange={(_event, checked) => setEditActive(checked)}
              label="Active"
              labelOff="Inactive"
            />
          </FormGroup>
        </Form>
      </Modal>
    </div>
  );
}
