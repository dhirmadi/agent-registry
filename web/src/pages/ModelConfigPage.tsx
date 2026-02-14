import { useEffect, useState, useCallback } from 'react';
import {
  ActionGroup,
  Alert,
  Button,
  Form,
  FormGroup,
  Slider,
  Spinner,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { api } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import type { ModelConfig } from '../types';

export function ModelConfigPage() {
  const { user } = useAuth();
  const canWrite = user?.role === 'admin' || user?.role === 'editor';

  const [config, setConfig] = useState<ModelConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);

  // Form fields
  const [defaultModel, setDefaultModel] = useState('');
  const [temperature, setTemperature] = useState(0.7);
  const [maxTokens, setMaxTokens] = useState('');
  const [maxToolRounds, setMaxToolRounds] = useState('');
  const [defaultContextWindow, setDefaultContextWindow] = useState('');
  const [defaultMaxOutputTokens, setDefaultMaxOutputTokens] = useState('');
  const [historyTokenBudget, setHistoryTokenBudget] = useState('');
  const [maxHistoryMessages, setMaxHistoryMessages] = useState('');
  const [embeddingModel, setEmbeddingModel] = useState('');

  const fetchConfig = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.get<ModelConfig>('/api/v1/config/model?scope=global');
      setConfig(data);
      setDefaultModel(data.default_model);
      setTemperature(data.temperature);
      setMaxTokens(String(data.max_tokens));
      setMaxToolRounds(String(data.max_tool_rounds));
      setDefaultContextWindow(String(data.default_context_window));
      setDefaultMaxOutputTokens(String(data.default_max_output_tokens));
      setHistoryTokenBudget(String(data.history_token_budget));
      setMaxHistoryMessages(String(data.max_history_messages));
      setEmbeddingModel(data.embedding_model);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load model config');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  async function handleSave() {
    if (!config) return;
    setSaveError(null);
    setSaveSuccess(false);
    try {
      await api.put(
        '/api/v1/config/model',
        {
          scope: 'global',
          default_model: defaultModel,
          temperature,
          max_tokens: Number(maxTokens),
          max_tool_rounds: Number(maxToolRounds),
          default_context_window: Number(defaultContextWindow),
          default_max_output_tokens: Number(defaultMaxOutputTokens),
          history_token_budget: Number(historyTokenBudget),
          max_history_messages: Number(maxHistoryMessages),
          embedding_model: embeddingModel,
        },
        config.updated_at,
      );
      setSaveSuccess(true);
      await fetchConfig();
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save model config');
    }
  }

  if (loading) {
    return (
      <div data-testid="model-config-loading" style={{ textAlign: 'center', padding: '3rem' }}>
        <Spinner aria-label="Loading model config" />
      </div>
    );
  }

  if (error) {
    return (
      <Alert variant="danger" title="Error loading model config" data-testid="model-config-error">
        {error}
      </Alert>
    );
  }

  return (
    <div>
      <Title headingLevel="h1" style={{ marginBottom: '1.5rem' }}>
        Model Configuration
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

      <Form data-testid="model-config-form">
        <FormGroup label="Default Model" isRequired fieldId="default-model">
          <TextInput
            id="default-model"
            value={defaultModel}
            onChange={(_event, val) => setDefaultModel(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label={`Temperature (${temperature.toFixed(2)})`} fieldId="temperature">
          <Slider
            id="temperature"
            value={temperature * 50}
            onChange={(_event, val) => setTemperature(val / 50)}
            max={100}
            min={0}
            aria-label="Temperature"
            isDisabled={!canWrite}
            data-testid="temperature-slider"
          />
        </FormGroup>

        <FormGroup label="Max Tokens" fieldId="max-tokens">
          <TextInput
            id="max-tokens"
            type="number"
            value={maxTokens}
            onChange={(_event, val) => setMaxTokens(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label="Max Tool Rounds" fieldId="max-tool-rounds">
          <TextInput
            id="max-tool-rounds"
            type="number"
            value={maxToolRounds}
            onChange={(_event, val) => setMaxToolRounds(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label="Default Context Window" fieldId="default-context-window">
          <TextInput
            id="default-context-window"
            type="number"
            value={defaultContextWindow}
            onChange={(_event, val) => setDefaultContextWindow(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label="Default Max Output Tokens" fieldId="default-max-output-tokens">
          <TextInput
            id="default-max-output-tokens"
            type="number"
            value={defaultMaxOutputTokens}
            onChange={(_event, val) => setDefaultMaxOutputTokens(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label="History Token Budget" fieldId="history-token-budget">
          <TextInput
            id="history-token-budget"
            type="number"
            value={historyTokenBudget}
            onChange={(_event, val) => setHistoryTokenBudget(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label="Max History Messages" fieldId="max-history-messages">
          <TextInput
            id="max-history-messages"
            type="number"
            value={maxHistoryMessages}
            onChange={(_event, val) => setMaxHistoryMessages(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        <FormGroup label="Embedding Model" fieldId="embedding-model">
          <TextInput
            id="embedding-model"
            value={embeddingModel}
            onChange={(_event, val) => setEmbeddingModel(val)}
            isDisabled={!canWrite}
          />
        </FormGroup>

        {canWrite && (
          <ActionGroup>
            <Button variant="primary" onClick={handleSave} data-testid="save-model-config-btn">
              Save
            </Button>
          </ActionGroup>
        )}
      </Form>
    </div>
  );
}
