import { request as httpsRequest } from 'node:https';
import { request as httpRequest, type Agent, type IncomingMessage } from 'node:http';

/** A fully prepared upstream HTTP request. */
export interface PreparedRequest {
  url: URL;
  headers: Record<string, string>;
  body: Buffer;
}

/**
 * Sends the request and resolves once upstream response headers arrive.
 * Rejects on connection errors, on `timeoutMs` of socket inactivity, or when `signal` aborts.
 */
export function sendUpstream(
  req: PreparedRequest,
  opts: { timeoutMs: number; signal: AbortSignal; agent?: Agent },
): Promise<IncomingMessage> {
  return new Promise((resolve, reject) => {
    const doRequest = req.url.protocol === 'https:' ? httpsRequest : httpRequest;
    const out = doRequest(req.url, {
      method: 'POST',
      headers: { ...req.headers, 'content-length': String(req.body.length) },
      timeout: opts.timeoutMs,
      signal: opts.signal,
      agent: opts.agent,
    }, resolve);
    out.on('timeout', () => out.destroy(new UpstreamTimeout()));
    out.on('error', reject);
    out.end(req.body);
  });
}

export class UpstreamTimeout extends Error {
  constructor() {
    super('upstream timeout');
  }
}

/**
 * Joins an API path onto a channel base URL, keeping any path prefix in the base
 * (e.g. https://openrouter.ai/api). A base that already ends in /v1 is not doubled.
 */
export function joinURL(baseURL: string, apiPath: string): URL {
  const url = new URL(baseURL);
  const base = url.pathname.replace(/\/+$/, '');
  const path = base.endsWith('/v1') && apiPath.startsWith('/v1/') ? apiPath.slice(3) : apiPath;
  url.pathname = base + path;
  return url;
}
