import { useState, useCallback } from 'react';
import { Button, Modal, TextInput } from '@patternfly/react-core';

export interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string;
  confirmText?: string;
  onConfirm: () => void;
  onCancel: () => void;
  variant?: 'danger' | 'warning' | 'default';
  resourceName?: string;
}

export function ConfirmDialog({
  isOpen,
  title,
  message,
  confirmText = 'Confirm',
  onConfirm,
  onCancel,
  variant = 'default',
  resourceName,
}: ConfirmDialogProps) {
  const [typedName, setTypedName] = useState('');

  const handleConfirm = useCallback(() => {
    setTypedName('');
    onConfirm();
  }, [onConfirm]);

  const handleCancel = useCallback(() => {
    setTypedName('');
    onCancel();
  }, [onCancel]);

  const confirmDisabled = resourceName ? typedName !== resourceName : false;

  const buttonVariant = variant === 'danger' ? 'danger' : variant === 'warning' ? 'warning' : 'primary';

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleCancel}
      title={title}
      titleIconVariant={variant === 'danger' ? 'danger' : variant === 'warning' ? 'warning' : undefined}
      variant="small"
      aria-label={title}
      actions={[
        <Button
          key="confirm"
          variant={buttonVariant}
          onClick={handleConfirm}
          isDisabled={confirmDisabled}
          data-testid="confirm-button"
        >
          {confirmText}
        </Button>,
        <Button
          key="cancel"
          variant="link"
          onClick={handleCancel}
          data-testid="cancel-button"
        >
          Cancel
        </Button>,
      ]}
    >
      <p>{message}</p>
      {resourceName && (
        <div style={{ marginTop: '1rem' }}>
          <p>
            Type <strong>{resourceName}</strong> to confirm:
          </p>
          <TextInput
            aria-label="Type resource name to confirm"
            value={typedName}
            onChange={(_event, value) => setTypedName(value)}
            data-testid="confirm-name-input"
          />
        </div>
      )}
    </Modal>
  );
}
