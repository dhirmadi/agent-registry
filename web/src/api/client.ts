import type { Envelope } from '../types';

/**
 * Read the CSRF token from the csrf cookie.
 * Over HTTPS the cookie is "__Host-csrf"; over plain HTTP it is "csrf".
 * We pick the name that matches the current protocol so a stale cookie
 * left over from a mode switch (HTTP ↔ HTTPS) is never used.
 * If duplicates exist we take the LAST occurrence (the freshest Set-Cookie wins).
 */
function getCSRFToken(): string {
  const isSecure = window.location.protocol === 'https:';
  const name = isSecure ? '__Host-csrf' : 'csrf';
  const prefix = name + '=';

  let value = '';
  for (const part of document.cookie.split('; ')) {
    if (part.startsWith(prefix)) {
      value = part.substring(prefix.length);
    }
  }
  return value;
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

  // Guard against non-JSON responses (e.g. chi's plain-text 404/405, HTML fallback).
  // Only reject when Content-Type is explicitly set to something other than JSON.
  const contentType = response.headers?.get?.('Content-Type') ?? '';
  if (contentType && !contentType.includes('application/json')) {
    const text = await response.text();
    throw new APIError(
      'NON_JSON_RESPONSE',
      text.trim() || `Unexpected ${response.status} response`,
      response.status,
    );
  }

  let envelope: Envelope<T>;
  try {
    envelope = await response.json();
  } catch {
    throw new APIError(
      'PARSE_ERROR',
      `Failed to parse response (HTTP ${response.status})`,
      response.status,
    );
  }

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
