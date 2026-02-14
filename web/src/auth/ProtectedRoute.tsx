import type { ReactNode } from 'react';
import { Navigate } from 'react-router-dom';
import { Spinner } from '@patternfly/react-core';
import { useAuth } from './AuthContext';

interface ProtectedRouteProps {
  children: ReactNode;
  allowMustChangePass?: boolean;
}

export function ProtectedRoute({ children, allowMustChangePass }: ProtectedRouteProps) {
  const { user, loading, mustChangePassword } = useAuth();

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <Spinner aria-label="Loading" />
      </div>
    );
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  if (mustChangePassword && !allowMustChangePass) {
    return <Navigate to="/change-password" replace />;
  }

  return <>{children}</>;
}
