import type { Envelope } from '../types';

/** Read the CSRF token from the __Host-csrf cookie. */
function getCSRFToken(): string {
  const match = document.cookie
    .split('; ')
    .find((row) => row.startsWith('__Host-csrf='));
  return match ? match.split('=')[1] : '';
}

export class APIError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = 'APIError';
  }
}

/**
 * Fetch wrapper that handles CSRF tokens, session cookies,
 * and the standard API envelope format.
 */
export async function apiFetch<T = unknown>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const headers = new Headers(options.headers);

  // Always send cookies (session)
  const method = (options.method || 'GET').toUpperCase();

  // Attach CSRF token for mutation requests
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    const csrf = getCSRFToken();
    if (csrf) {
      headers.set('X-CSRF-Token', csrf);
    }
  }

  // Default content type for requests with body
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(path, {
    ...options,
    headers,
    credentials: 'same-origin',
  });

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  const envelope: Envelope<T> = await response.json();

  if (!envelope.success) {
    const err = envelope.error;
    throw new APIError(
      err?.code || 'UNKNOWN',
      err?.message || 'An unknown error occurred',
      response.status,
    );
  }

  return envelope.data;
}

/** Convenience helpers */
export const api = {
  get: <T>(path: string) => apiFetch<T>(path),

  post: <T>(path: string, body?: unknown) =>
    apiFetch<T>(path, {
      method: 'POST',
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }),

  put: <T>(path: string, body: unknown, etag?: string) => {
    const headers: Record<string, string> = {};
    if (etag) {
      headers['If-Match'] = etag;
    }
    return apiFetch<T>(path, {
      method: 'PUT',
      body: JSON.stringify(body),
      headers,
    });
  },

  patch: <T>(path: string, body: unknown, etag?: string) => {
    const headers: Record<string, string> = {};
    if (etag) {
      headers['If-Match'] = etag;
    }
    return apiFetch<T>(path, {
      method: 'PATCH',
      body: JSON.stringify(body),
      headers,
    });
  },

  delete: <T = void>(path: string) =>
    apiFetch<T>(path, { method: 'DELETE' }),
};
