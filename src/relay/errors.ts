import type { Response } from 'express';

/** Which API shape the client spoke — errors are returned in the same shape. */
export type InboundAPI = 'openai' | 'anthropic';

/** Thrown while preparing a request when the client sent something invalid. */
export class RelayError extends Error {
  constructor(readonly status: number, readonly type: string, message: string) {
    super(message);
  }
}

export function sendError(res: Response, api: InboundAPI, status: number, type: string, message: string): void {
  if (res.headersSent) {
    res.end();
    return;
  }
  const body = api === 'anthropic'
    ? { type: 'error', error: { type, message } }
    : { error: { message, type, code: type } };
  res.status(status).json(body);
}
