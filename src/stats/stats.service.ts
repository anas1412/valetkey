import { Injectable } from '@nestjs/common';

/** One relayed request, as shown in the dashboard's activity log. Never holds prompt or response content. */
export interface RequestEntry {
  time: number;
  namespace: string;
  key: string;
  group: string;
  channel: string;
  model: string;
  status: number;
  inputTokens: number;
  outputTokens: number;
  latencyMs: number;
  error?: string;
}

/** An operator event (key minted, channel lost its key, reconcile error…). */
export interface OperatorEvent {
  time: number;
  level: 'info' | 'warn' | 'error';
  kind: string;
  message: string;
}

/** Per-minute totals for the overview charts. */
export interface MinuteBucket {
  minute: number; // epoch minutes
  requests: number;
  errors: number;
  inputTokens: number;
  outputTokens: number;
}

const MAX_REQUESTS = 500;
const MAX_EVENTS = 200;
const MINUTES = 24 * 60;

/**
 * In-memory usage history for the dashboard. No database: everything resets
 * when the pod restarts, and each replica only sees its own traffic.
 */
@Injectable()
export class StatsService {
  readonly startedAt = Date.now();
  private requests: RequestEntry[] = [];
  private events: OperatorEvent[] = [];
  private buckets = new Map<number, MinuteBucket>();
  /** ProxyKey "ns/name" → last request time */
  private lastUsed = new Map<string, number>();
  private keyRequests = new Map<string, number>();

  recordRequest(entry: RequestEntry): void {
    push(this.requests, entry, MAX_REQUESTS);
    const id = `${entry.namespace}/${entry.key}`;
    this.lastUsed.set(id, entry.time);
    this.keyRequests.set(id, (this.keyRequests.get(id) ?? 0) + 1);

    const minute = Math.floor(entry.time / 60_000);
    let b = this.buckets.get(minute);
    if (!b) {
      b = { minute, requests: 0, errors: 0, inputTokens: 0, outputTokens: 0 };
      this.buckets.set(minute, b);
      for (const m of this.buckets.keys()) if (m <= minute - MINUTES) this.buckets.delete(m);
    }
    b.requests++;
    if (entry.status >= 400) b.errors++;
    b.inputTokens += entry.inputTokens;
    b.outputTokens += entry.outputTokens;
  }

  event(level: OperatorEvent['level'], kind: string, message: string): void {
    push(this.events, { time: Date.now(), level, kind, message }, MAX_EVENTS);
  }

  recentRequests(): RequestEntry[] {
    return [...this.requests].reverse();
  }

  recentEvents(): OperatorEvent[] {
    return [...this.events].reverse();
  }

  /** Per-minute buckets for the last 24 h, oldest first, with empty minutes filled in. */
  series(): MinuteBucket[] {
    const now = Math.floor(Date.now() / 60_000);
    const out: MinuteBucket[] = [];
    for (let m = now - MINUTES + 1; m <= now; m++) {
      out.push(this.buckets.get(m) ?? { minute: m, requests: 0, errors: 0, inputTokens: 0, outputTokens: 0 });
    }
    return out;
  }

  keyUsage(namespace: string, name: string): { requests: number; lastUsed?: number } {
    const id = `${namespace}/${name}`;
    return { requests: this.keyRequests.get(id) ?? 0, lastUsed: this.lastUsed.get(id) };
  }

  /** Requests per channel over the kept request log, for the traffic-share chart. */
  channelShare(): Record<string, number> {
    const share: Record<string, number> = {};
    for (const r of this.requests) if (r.channel !== 'none') share[r.channel] = (share[r.channel] ?? 0) + 1;
    return share;
  }
}

function push<T>(list: T[], item: T, max: number): void {
  list.push(item);
  if (list.length > max) list.shift();
}
