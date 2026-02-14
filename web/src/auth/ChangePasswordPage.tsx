import { useState } from 'react';
import type { FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  PageSection,
  Card,
  CardBody,
  Form,
  FormGroup,
  TextInput,
  ActionGroup,
  Button,
  Alert,
  Title,
  HelperText,
  HelperTextItem,
} from '@patternfly/react-core';
import { api } from '../api/client';
import { useAuth } from './AuthContext';

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

export function ChangePasswordPage() {
  const navigate = useNavigate();
  const { refreshUser } = useAuth();

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const validationErrors = newPassword ? validatePassword(newPassword) : [];
  const passwordsMatch = newPassword === confirmPassword;
  const canSubmit =
    currentPassword &&
    newPassword &&
    confirmPassword &&
    validationErrors.length === 0 &&
    passwordsMatch &&
    !isSubmitting;

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    setError('');
    setIsSubmitting(true);

    try {
      await api.post('/auth/change-password', {
        current_password: currentPassword,
        new_password: newPassword,
      });
      // Refresh auth context so mustChangePassword is cleared before navigating.
      // The session is preserved server-side (only other sessions are invalidated).
      await refreshUser();
      navigate('/', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to change password');
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <PageSection>
      <Card style={{ maxWidth: 500, margin: '0 auto' }}>
        <CardBody>
          <Title headingLevel="h1" size="xl" style={{ marginBottom: '1rem' }}>
            Change Password
          </Title>

          {error && (
            <Alert variant="danger" title={error} isInline style={{ marginBottom: '1rem' }} />
          )}

          <Form onSubmit={handleSubmit}>
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

            <ActionGroup>
              <Button
                type="submit"
                variant="primary"
                isDisabled={!canSubmit}
                isLoading={isSubmitting}
              >
                Change Password
              </Button>
            </ActionGroup>
          </Form>
        </CardBody>
      </Card>
    </PageSection>
  );
}
