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
import type { TrustDefault, TrustRule } from '../types';

interface TrustDefaultsResponse {
  items: TrustDefault[];
  total: number;
}

interface TrustRulesResponse {
  items: TrustRule[];
  total: number;
}

const TIER_OPTIONS: TrustDefault['tier'][] = ['auto', 'review', 'block'];

export function TrustPage() {
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  // System Defaults
  const [defaults, setDefaults] = useState<TrustDefault[]>([]);
  const [defaultsLoading, setDefaultsLoading] = useState(true);
  const [defaultsError, setDefaultsError] = useState<string | null>(null);
  const [createDefaultOpen, setCreateDefaultOpen] = useState(false);
  const [newDefaultTier, setNewDefaultTier] = useState<TrustDefault['tier']>('auto');
  const [newDefaultPatterns, setNewDefaultPatterns] = useState('');
  const [newDefaultPriority, setNewDefaultPriority] = useState('0');
  const [deleteDefaultId, setDeleteDefaultId] = useState<string | null>(null);

  // Workspace Rules
  const [rules, setRules] = useState<TrustRule[]>([]);
  const [rulesLoading, setRulesLoading] = useState(false);
  const [rulesError, setRulesError] = useState<string | null>(null);
  const [workspaceId, setWorkspaceId] = useState('');
  const [createRuleOpen, setCreateRuleOpen] = useState(false);
  const [newRulePattern, setNewRulePattern] = useState('');
  const [newRuleTier, setNewRuleTier] = useState<TrustRule['tier']>('auto');
  const [deleteRuleId, setDeleteRuleId] = useState<string | null>(null);

  const fetchDefaults = useCallback(async () => {
    setDefaultsLoading(true);
    setDefaultsError(null);
    try {
      const data = await api.get<TrustDefaultsResponse>('/api/v1/trust/defaults');
      setDefaults(data.items ?? []);
    } catch (err) {
      setDefaultsError(err instanceof Error ? err.message : 'Failed to load trust defaults');
    } finally {
      setDefaultsLoading(false);
    }
  }, []);

  const fetchRules = useCallback(async (wsId: string) => {
    if (!wsId) {
      setRules([]);
      return;
    }
    setRulesLoading(true);
    setRulesError(null);
    try {
      const data = await api.get<TrustRulesResponse>(`/api/v1/trust/rules?workspace_id=${encodeURIComponent(wsId)}`);
      setRules(data.items ?? []);
    } catch (err) {
      setRulesError(err instanceof Error ? err.message : 'Failed to load trust rules');
    } finally {
      setRulesLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDefaults();
  }, [fetchDefaults]);

  async function handleCreateDefault() {
    try {
      await api.post('/api/v1/trust/defaults', {
        tier: newDefaultTier,
        patterns: newDefaultPatterns.split(',').map((p) => p.trim()).filter(Boolean),
        priority: Number(newDefaultPriority),
      });
      setCreateDefaultOpen(false);
      setNewDefaultTier('auto');
      setNewDefaultPatterns('');
      setNewDefaultPriority('0');
      await fetchDefaults();
    } catch {
      // Error handled by refetch
    }
  }

  async function handleDeleteDefault(id: string) {
    try {
      await api.delete(`/api/v1/trust/defaults/${id}`);
      await fetchDefaults();
    } catch {
      // Error handled by refetch
    }
    setDeleteDefaultId(null);
  }

  async function handleSearchRules() {
    await fetchRules(workspaceId);
  }

  async function handleCreateRule() {
    try {
      await api.post('/api/v1/trust/rules', {
        workspace_id: workspaceId,
        tool_pattern: newRulePattern,
        tier: newRuleTier,
      });
      setCreateRuleOpen(false);
      setNewRulePattern('');
      setNewRuleTier('auto');
      await fetchRules(workspaceId);
    } catch {
      // Error handled by refetch
    }
  }

  async function handleDeleteRule(id: string) {
    try {
      await api.delete(`/api/v1/trust/rules/${id}`);
      await fetchRules(workspaceId);
    } catch {
      // Error handled by refetch
    }
    setDeleteRuleId(null);
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Trust Management
      </Title>

      {/* System Defaults Section */}
      <Title headingLevel="h2" style={{ marginBottom: '1rem' }}>
        System Defaults
      </Title>

      {defaultsLoading ? (
        <div data-testid="defaults-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading trust defaults" />
        </div>
      ) : defaultsError ? (
        <Alert variant="danger" title="Error loading trust defaults" data-testid="defaults-error">
          {defaultsError}
        </Alert>
      ) : (
        <>
          {canWrite && (
            <Toolbar>
              <ToolbarContent>
                <ToolbarItem>
                  <Button variant="primary" onClick={() => setCreateDefaultOpen(true)} data-testid="add-default-btn">
                    Add Default
                  </Button>
                </ToolbarItem>
              </ToolbarContent>
            </Toolbar>
          )}

          {defaults.length === 0 ? (
            <Alert variant="info" title="No trust defaults configured" isInline isPlain />
          ) : (
            <Table aria-label="Trust defaults table" data-testid="defaults-table">
              <Thead>
                <Tr>
                  <Th>Tier</Th>
                  <Th>Patterns</Th>
                  <Th>Priority</Th>
                  {canWrite && <Th>Actions</Th>}
                </Tr>
              </Thead>
              <Tbody>
                {defaults.map((d) => (
                  <Tr key={d.id}>
                    <Td dataLabel="Tier">{d.tier}</Td>
                    <Td dataLabel="Patterns">{d.patterns.join(', ')}</Td>
                    <Td dataLabel="Priority">{d.priority}</Td>
                    {canWrite && (
                      <Td>
                        <Button
                          variant="danger"
                          size="sm"
                          onClick={() => setDeleteDefaultId(d.id)}
                          data-testid={`delete-default-${d.id}`}
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
        </>
      )}

      {/* Workspace Rules Section */}
      <Title headingLevel="h2" style={{ marginTop: '2rem', marginBottom: '1rem' }}>
        Workspace Rules
      </Title>

      <Toolbar>
        <ToolbarContent>
          <ToolbarItem>
            <TextInput
              aria-label="Workspace ID"
              placeholder="Enter workspace ID"
              value={workspaceId}
              onChange={(_event, val) => setWorkspaceId(val)}
              data-testid="workspace-id-input"
            />
          </ToolbarItem>
          <ToolbarItem>
            <Button variant="secondary" onClick={handleSearchRules} data-testid="search-rules-btn">
              Search
            </Button>
          </ToolbarItem>
          {canWrite && workspaceId && (
            <ToolbarItem>
              <Button variant="primary" onClick={() => setCreateRuleOpen(true)} data-testid="add-rule-btn">
                Add Rule
              </Button>
            </ToolbarItem>
          )}
        </ToolbarContent>
      </Toolbar>

      {rulesLoading ? (
        <div data-testid="rules-loading" style={{ textAlign: 'center', padding: '2rem' }}>
          <Spinner aria-label="Loading trust rules" />
        </div>
      ) : rulesError ? (
        <Alert variant="danger" title="Error loading trust rules" data-testid="rules-error">
          {rulesError}
        </Alert>
      ) : rules.length > 0 ? (
        <Table aria-label="Trust rules table" data-testid="rules-table">
          <Thead>
            <Tr>
              <Th>Tool Pattern</Th>
              <Th>Tier</Th>
              <Th>Created By</Th>
              {canWrite && <Th>Actions</Th>}
            </Tr>
          </Thead>
          <Tbody>
            {rules.map((r) => (
              <Tr key={r.id}>
                <Td dataLabel="Tool Pattern">{r.tool_pattern}</Td>
                <Td dataLabel="Tier">{r.tier}</Td>
                <Td dataLabel="Created By">{r.created_by}</Td>
                {canWrite && (
                  <Td>
                    <Button
                      variant="danger"
                      size="sm"
                      onClick={() => setDeleteRuleId(r.id)}
                      data-testid={`delete-rule-${r.id}`}
                    >
                      Delete
                    </Button>
                  </Td>
                )}
              </Tr>
            ))}
          </Tbody>
        </Table>
      ) : workspaceId ? (
        <Alert variant="info" title="No rules found for this workspace" isInline isPlain />
      ) : null}

      {/* Create Default Modal */}
      <Modal
        isOpen={createDefaultOpen}
        onClose={() => setCreateDefaultOpen(false)}
        title="Add Trust Default"
        variant="medium"
        aria-label="Add Trust Default"
        actions={[
          <Button key="submit" variant="primary" onClick={handleCreateDefault} data-testid="submit-default-btn">
            Add
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setCreateDefaultOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <FormGroup label="Tier" isRequired fieldId="default-tier">
            <FormSelect
              id="default-tier"
              value={newDefaultTier}
              onChange={(_event, val) => setNewDefaultTier(val as TrustDefault['tier'])}
              aria-label="Tier"
            >
              {TIER_OPTIONS.map((t) => (
                <FormSelectOption key={t} value={t} label={t} />
              ))}
            </FormSelect>
          </FormGroup>
          <FormGroup label="Patterns (comma-separated)" isRequired fieldId="default-patterns">
            <TextInput
              id="default-patterns"
              value={newDefaultPatterns}
              onChange={(_event, val) => setNewDefaultPatterns(val)}
              placeholder="e.g., git_*, slack_send_message"
            />
          </FormGroup>
          <FormGroup label="Priority" fieldId="default-priority">
            <TextInput
              id="default-priority"
              type="number"
              value={newDefaultPriority}
              onChange={(_event, val) => setNewDefaultPriority(val)}
            />
          </FormGroup>
        </Form>
      </Modal>

      {/* Create Rule Modal */}
      <Modal
        isOpen={createRuleOpen}
        onClose={() => setCreateRuleOpen(false)}
        title="Add Trust Rule"
        variant="medium"
        aria-label="Add Trust Rule"
        actions={[
          <Button key="submit" variant="primary" onClick={handleCreateRule} data-testid="submit-rule-btn">
            Add
          </Button>,
          <Button key="cancel" variant="link" onClick={() => setCreateRuleOpen(false)}>
            Cancel
          </Button>,
        ]}
      >
        <Form>
          <FormGroup label="Tool Pattern" isRequired fieldId="rule-pattern">
            <TextInput
              id="rule-pattern"
              value={newRulePattern}
              onChange={(_event, val) => setNewRulePattern(val)}
              placeholder="e.g., git_read_*"
            />
          </FormGroup>
          <FormGroup label="Tier" isRequired fieldId="rule-tier">
            <FormSelect
              id="rule-tier"
              value={newRuleTier}
              onChange={(_event, val) => setNewRuleTier(val as TrustRule['tier'])}
              aria-label="Tier"
            >
              {TIER_OPTIONS.map((t) => (
                <FormSelectOption key={t} value={t} label={t} />
              ))}
            </FormSelect>
          </FormGroup>
        </Form>
      </Modal>

      {/* Delete Default Confirmation */}
      <ConfirmDialog
        isOpen={deleteDefaultId !== null}
        title="Delete Trust Default"
        message="Are you sure you want to delete this trust default?"
        confirmText="Delete"
        variant="danger"
        onConfirm={() => deleteDefaultId && handleDeleteDefault(deleteDefaultId)}
        onCancel={() => setDeleteDefaultId(null)}
      />

      {/* Delete Rule Confirmation */}
      <ConfirmDialog
        isOpen={deleteRuleId !== null}
        title="Delete Trust Rule"
        message="Are you sure you want to delete this trust rule?"
        confirmText="Delete"
        variant="danger"
        onConfirm={() => deleteRuleId && handleDeleteRule(deleteRuleId)}
        onCancel={() => setDeleteRuleId(null)}
      />
    </div>
  );
}
