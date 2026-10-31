import { HttpException } from '@nestjs/common';

/**
 * Runs a Kubernetes API call and turns its failures into HTTP errors with the
 * API server's message (e.g. schema validation, already exists, not found).
 */
export async function k8sCall<T>(fn: () => Promise<T>): Promise<T> {
  try {
    return await fn();
  } catch (err) {
    const e = err as { code?: number; body?: unknown; message?: string };
    if (typeof e.code !== 'number') throw err;
    let message = e.message ?? 'kubernetes API error';
    try {
      const body = typeof e.body === 'string' ? JSON.parse(e.body) : e.body;
      if (body?.message) message = body.message;
    } catch {}
    throw new HttpException({ message }, e.code >= 400 && e.code < 600 ? e.code : 502);
  }
}

/** Validates a Kubernetes object name (DNS-1123 subdomain). */
export function assertName(name: unknown, what: string): asserts name is string {
  if (typeof name !== 'string' || !/^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$/.test(name)) {
    throw new HttpException({ message: `${what} must be lowercase letters, digits, "-" or "." (e.g. "my-team")` }, 400);
  }
}
