import type { PreparedRequest } from '../upstream.js';

/** Incrementally converts one SSE stream format into another. */
export interface StreamTranslator {
  /** Feed a raw upstream chunk, get the SSE text to send to the client. */
  push(chunk: string): string;
  /** Called once at end of stream; returns any remaining output. */
  end(): string;
}

/** The result of adapting a client request for a specific channel. */
export interface Adaptation {
  request: PreparedRequest;
  /** Converts a buffered upstream JSON body to the client's shape. Absent = pass through. */
  translateJSON?: (body: unknown, status: number) => unknown;
  /** Converts upstream SSE to the client's shape. Absent = pass through. */
  createStreamTranslator?: () => StreamTranslator;
}
