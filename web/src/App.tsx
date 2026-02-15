import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { AuthProvider } from './auth/AuthContext';
import { ProtectedRoute } from './auth/ProtectedRoute';
import { ToastProvider } from './components/ToastNotifications';
import { ErrorBoundary } from './components/ErrorBoundary';
import { LoginPage } from './auth/LoginPage';
import { ChangePasswordPage } from './auth/ChangePasswordPage';
import { AppLayout } from './components/AppLayout';
import { DashboardPage } from './pages/DashboardPage';
import { AgentsPage } from './pages/AgentsPage';
import { AgentDetailPage } from './pages/AgentDetailPage';
import { PromptsPage } from './pages/PromptsPage';
import { MCPServersPage } from './pages/MCPServersPage';
import { TrustPage } from './pages/TrustPage';
import { ModelConfigPage } from './pages/ModelConfigPage';
import { WebhooksPage } from './pages/WebhooksPage';
import { APIKeysPage } from './pages/APIKeysPage';
import { UsersPage } from './pages/UsersPage';
import { AuditLogPage } from './pages/AuditLogPage';
import { MyAccountPage } from './pages/MyAccountPage';

export function App() {
  return (
    <BrowserRouter>
      <ErrorBoundary>
        <AuthProvider>
          <ToastProvider>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route
                path="/change-password"
                element={
                  <ProtectedRoute allowMustChangePass>
                    <ChangePasswordPage />
                  </ProtectedRoute>
                }
              />
              <Route
                path="/"
                element={
                  <ProtectedRoute>
                    <AppLayout />
                  </ProtectedRoute>
                }
              >
                <Route index element={<DashboardPage />} />
                <Route path="agents" element={<AgentsPage />} />
                <Route path="agents/:agentId" element={<AgentDetailPage />} />
                <Route path="prompts" element={<PromptsPage />} />
                <Route path="mcp-servers" element={<MCPServersPage />} />
                <Route path="trust-rules" element={<TrustPage />} />
                <Route path="model-config" element={<ModelConfigPage />} />
                <Route path="webhooks" element={<WebhooksPage />} />
                <Route path="api-keys" element={<APIKeysPage />} />
                <Route path="users" element={<UsersPage />} />
                <Route path="audit-log" element={<AuditLogPage />} />
                <Route path="my-account" element={<MyAccountPage />} />
              </Route>
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </ToastProvider>
        </AuthProvider>
      </ErrorBoundary>
    </BrowserRouter>
  );
}
