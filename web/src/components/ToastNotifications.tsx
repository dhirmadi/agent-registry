import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
} from 'react';
import type { ReactNode } from 'react';
import {
  Alert,
  AlertActionCloseButton,
  AlertGroup,
} from '@patternfly/react-core';

type ToastVariant = 'success' | 'danger' | 'warning' | 'info';

interface Toast {
  id: number;
  variant: ToastVariant;
  title: string;
  message?: string;
}

interface ToastContextValue {
  addToast: (variant: ToastVariant, title: string, message?: string) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used within a ToastProvider');
  }
  return ctx;
}

interface ToastProviderProps {
  children: ReactNode;
}

export function ToastProvider({ children }: ToastProviderProps) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(0);

  const removeToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const addToast = useCallback(
    (variant: ToastVariant, title: string, message?: string) => {
      const id = nextId.current++;
      setToasts((prev) => [...prev, { id, variant, title, message }]);
    },
    [removeToast],
  );

  const value = useMemo(() => ({ addToast }), [addToast]);

  return (
    <ToastContext.Provider value={value}>
      {children}
      <AlertGroup isToast isLiveRegion aria-label="Notifications">
        {toasts.map((toast) => (
          <Alert
            key={toast.id}
            variant={toast.variant}
            title={toast.title}
            actionClose={
              <AlertActionCloseButton onClose={() => removeToast(toast.id)} />
            }
            timeout={5000}
            onTimeout={() => removeToast(toast.id)}
          >
            {toast.message}
          </Alert>
        ))}
      </AlertGroup>
    </ToastContext.Provider>
  );
}
