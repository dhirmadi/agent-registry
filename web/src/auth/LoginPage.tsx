import { useState } from 'react';
import type { FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  LoginPage as PFLoginPage,
  LoginForm,
  ListVariant,
} from '@patternfly/react-core';
import { useAuth } from './AuthContext';

export function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    try {
      const data = await login(username, password);
      if (data.must_change_password) {
        navigate('/change-password', { replace: true });
      } else {
        navigate('/', { replace: true });
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Login failed');
    } finally {
      setIsLoading(false);
    }
  }

  const socialMediaLoginContent = (
    <a href="/auth/google/start">Sign in with Google</a>
  );

  const loginForm = (
    <LoginForm
      showHelperText={!!error}
      helperText={error}
      helperTextIcon={undefined}
      usernameLabel="Username"
      usernameValue={username}
      onChangeUsername={(_e, val) => setUsername(val)}
      passwordLabel="Password"
      passwordValue={password}
      onChangePassword={(_e, val) => setPassword(val)}
      isShowPasswordEnabled
      showPasswordAriaLabel="Show password"
      hidePasswordAriaLabel="Hide password"
      onLoginButtonClick={handleSubmit}
      loginButtonLabel={isLoading ? 'Signing in...' : 'Sign in'}
      isLoginButtonDisabled={isLoading || !username || !password}
    />
  );

  return (
    <PFLoginPage
      loginTitle="Sign in to Agentic Registry"
      loginSubtitle="Enter your credentials"
      socialMediaLoginContent={socialMediaLoginContent}
      socialMediaLoginAriaLabel="Other login methods"
      textContent=""
      footerListVariants={ListVariant.inline}
    >
      {loginForm}
    </PFLoginPage>
  );
}
