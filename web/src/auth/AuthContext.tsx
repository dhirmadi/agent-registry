import { createContext, useContext, useState, useEffect, useCallback } from 'react';
import type { ReactNode } from 'react';
import { api } from '../api/client';
import type { User, LoginResponse } from '../types';

interface AuthContextValue {
  user: User | null;
  loading: boolean;
  mustChangePassword: boolean;
  login: (username: string, password: string) => Promise<LoginResponse>;
  logout: () => Promise<void>;
  refreshUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [mustChangePassword, setMustChangePassword] = useState(false);

  useEffect(() => {
    api.get<User>('/auth/me')
      .then((u) => {
        setUser(u);
        setMustChangePassword(u.must_change_password ?? false);
      })
      .catch(() => {
        setUser(null);
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    const data = await api.post<LoginResponse>('/auth/login', { username, password });
    setUser(data.user);
    setMustChangePassword(data.must_change_password);
    return data;
  }, []);

  const logout = useCallback(async () => {
    await api.post('/auth/logout');
    setUser(null);
    setMustChangePassword(false);
  }, []);

  const refreshUser = useCallback(async () => {
    const u = await api.get<User>('/auth/me');
    setUser(u);
    setMustChangePassword(u.must_change_password ?? false);
  }, []);

  return (
    <AuthContext.Provider value={{ user, loading, mustChangePassword, login, logout, refreshUser }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return ctx;
}
