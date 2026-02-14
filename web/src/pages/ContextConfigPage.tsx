import { useEffect, useState, useCallback } from 'react';
import {
  ActionGroup,
  Alert,
  Button,
  Form,
  FormGroup,
  Label,
  Spinner,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import type { ContextConfig } from '../types';

export function ContextConfigPage() {
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [config, setConfig] = useState<ContextConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);

  // Form fields
  const [maxTotalTokens, setMaxTotalTokens] = useState('');
  const [layerBudgets, setLayerBudgets] = useState<Record<string, number>>({});
  const [enabledLayers, setEnabledLayers] = useState<string[]>([]);

  // For adding new layer budgets
  const [newLayerKey, setNewLayerKey] = useState('');
  const [newLayerValue, setNewLayerValue] = useState('');

  // For adding new enabled layers
  const [newEnabledLayer, setNewEnabledLayer] = useState('');

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<ContextConfig>('/api/v1/context-config');
      setConfig(data);
      setMaxTotalTokens(String(data.max_total_tokens));
      setLayerBudgets({ ...data.layer_budgets });
      setEnabledLayers([...data.enabled_layers]);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load context config');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  function handleAddLayerBudget() {
    if (!newLayerKey || !newLayerValue) return;
    setLayerBudgets((prev) => ({ ...prev, [newLayerKey]: Number(newLayerValue) }));
    setNewLayerKey('');
    setNewLayerValue('');
  }

  function handleRemoveLayerBudget(key: string) {
    setLayerBudgets((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function handleAddEnabledLayer() {
    if (!newEnabledLayer || enabledLayers.includes(newEnabledLayer)) return;
    setEnabledLayers((prev) => [...prev, newEnabledLayer]);
    setNewEnabledLayer('');
  }

  function handleRemoveEnabledLayer(layer: string) {
    setEnabledLayers((prev) => prev.filter((l) => l !== layer));
  }

  async function handleSave() {
    if (!config) return;
    setSaveError(null);
    setSaveSuccess(false);
    try {
      await api.put(
        '/api/v1/context-config',
        {
          scope: 'global',
          max_total_tokens: Number(maxTotalTokens),
          layer_budgets: layerBudgets,
          enabled_layers: enabledLayers,
        },
        config.updated_at,
      );
      setSaveSuccess(true);
      await fetchConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save context config');
    }
  }

  if (loading) {
    return (
      <div data-testid="context-config-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading context config" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading context config" data-testid="context-config-error">
        {error}
      </Alert>
    );
  }

  const layerBudgetEntries = Object.entries(layerBudgets);

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Context Configuration
      </Title>

      {saveSuccess && (
        <Alert
          variant="success"
          title="Configuration saved successfully"
          data-testid="save-success"
          isInline
          style={{ marginBottom: '1rem' }}
        />
      )}
      {saveError && (
        <Alert
          variant="danger"
          title="Failed to save"
          data-testid="save-error"
          isInline
          style={{ marginBottom: '1rem' }}
        >
          {saveError}
        </Alert>
      )}

      <Form data-testid="context-config-form">
        <FormGroup label="Max Total Tokens" isRequired fieldId="max-total-tokens">
          <TextInput
            id="max-total-tokens"
            type="number"
            value={maxTotalTokens}
            onChange={(_event, val) => setMaxTotalTokens(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        {/* Layer Budgets */}
        <FormGroup label="Layer Budgets" fieldId="layer-budgets">
          {layerBudgetEntries.length > 0 && (
            <Table aria-label="Layer budgets" data-testid="layer-budgets-table" variant="compact">
              <Thead>
                <Tr>
                  <Th>Layer</Th>
                  <Th>Budget</Th>
                  {canWrite && <Th>Actions</Th>}
                </Tr>
              </Thead>
              <Tbody>
                {layerBudgetEntries.map(([key, val]) => (
                  <Tr key={key}>
                    <Td>{key}</Td>
                    <Td>{val}</Td>
                    {canWrite && (
                      <Td>
                        <Button
                          variant="link"
                          size="sm"
                          onClick={() => handleRemoveLayerBudget(key)}
                          data-testid={`remove-budget-${key}`}
                        >
                          Remove
                        </Button>
                      </Td>
                    )}
                  </Tr>
                ))}
              </Tbody>
            </Table>
          )}
          {canWrite && (
            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
              <TextInput
                aria-label="Layer name"
                placeholder="Layer name"
                value={newLayerKey}
                onChange={(_event, val) => setNewLayerKey(val)}
                data-testid="new-layer-key"
              />
              <TextInput
                aria-label="Budget value"
                placeholder="Budget"
                type="number"
                value={newLayerValue}
                onChange={(_event, val) => setNewLayerValue(val)}
                data-testid="new-layer-value"
              />
              <Button
                variant="secondary"
                onClick={handleAddLayerBudget}
                data-testid="add-layer-budget-btn"
              >
                Add
              </Button>
            </div>
          )}
        </FormGroup>

        {/* Enabled Layers */}
        <FormGroup label="Enabled Layers" fieldId="enabled-layers">
          <div data-testid="enabled-layers-group" style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
            {enabledLayers.map((layer) => (
              <Label
                key={layer}
                color="blue"
                onClose={canWrite ? () => handleRemoveEnabledLayer(layer) : undefined}
                data-testid={`enabled-layer-${layer}`}
              >
                {layer}
              </Label>
            ))}
          </div>
          {canWrite && (
            <div style={{ display: 'flex', gap: '0.5rem', marginTop: '0.5rem' }}>
              <TextInput
                aria-label="New layer"
                placeholder="Layer name"
                value={newEnabledLayer}
                onChange={(_event, val) => setNewEnabledLayer(val)}
                data-testid="new-enabled-layer"
              />
              <Button
                variant="secondary"
                onClick={handleAddEnabledLayer}
                data-testid="add-enabled-layer-btn"
              >
                Add
              </Button>
            </div>
          )}
        </FormGroup>

        {canWrite && (
          <ActionGroup>
            <Button variant="primary" onClick={handleSave} data-testid="save-context-config-btn">
              Save
            </Button>
          </ActionGroup>
        )}
      </Form>
    </div>
  );
}
