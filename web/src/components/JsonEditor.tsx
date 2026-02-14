import { useState, useCallback } from 'react';
import {
  FormGroup,
  HelperText,
  HelperTextItem,
  TextArea,
} from '@patternfly/react-core';

interface JsonEditorProps {
  value: string;
  onChange: (value: string) => void;
  label?: string;
  readOnly?: boolean;
}

export function JsonEditor({
  value,
  onChange,
  label = 'JSON',
  readOnly = false,
}: JsonEditorProps) {
  const [error, setError] = useState<string | null>(null);

  const handleChange = useCallback(
    (_event: React.ChangeEvent<HTMLTextAreaElement>, val: string) => {
      onChange(val);
      try {
        JSON.parse(val);
        setError(null);
      } catch {
        // Don't set error on every keystroke - validate on blur
      }
    },
    [onChange],
  );

  const handleBlur = useCallback(() => {
    if (!value.trim()) {
      setError(null);
      return;
    }
    try {
      const parsed = JSON.parse(value);
      const pretty = JSON.stringify(parsed, null, 2);
      if (pretty !== value) {
        onChange(pretty);
      }
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Invalid JSON');
    }
  }, [value, onChange]);

  return (
    <FormGroup label={label} fieldId="json-editor">
      <TextArea
        id="json-editor"
        value={value}
        onChange={handleChange}
        onBlur={handleBlur}
        readOnlyVariant={readOnly ? 'default' : undefined}
        aria-label={label}
        resizeOrientation="vertical"
        rows={10}
        style={{ fontFamily: 'monospace' }}
        validated={error ? 'error' : 'default'}
      />
      {error && (
        <HelperText>
          <HelperTextItem variant="error">{error}</HelperTextItem>
        </HelperText>
      )}
    </FormGroup>
  );
}
