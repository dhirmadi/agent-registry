import { Component } from 'react';
import type { ReactNode, ErrorInfo } from 'react';
import { Alert, Button } from '@patternfly/react-core';

interface Props { children: ReactNode }
interface State { hasError: boolean; error: Error | null }

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false, error: null };
  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }
  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('ErrorBoundary caught:', error, info);
  }
  render() {
    if (this.state.hasError) {
      return (
        <div style={{ padding: '2rem' }}>
          <Alert variant="danger" title="Something went wrong">
            {this.state.error?.message}
          </Alert>
          <Button variant="primary" onClick={() => window.location.reload()} style={{ marginTop: '1rem' }}>
            Reload Page
          </Button>
        </div>
      );
    }
    return this.props.children;
  }
}
