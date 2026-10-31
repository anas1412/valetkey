import { Injectable } from '@nestjs/common';

interface Window {
  count: number;
  resetAt: number; // epoch ms
}

const WINDOW_MS = 60_000;

/**
 * In-memory fixed-window (1 minute) rate limiter, per ProxyKey. No Redis, so
 * limits apply per replica.
 */
@Injectable()
export class RateLimiterService {
  private rpm = new Map<string, Window>();
  private tpm = new Map<string, Window>();

  /**
   * Checks RPM and the remaining TPM budget for a key and, if allowed, counts the request.
   * Limits ≤ 0 mean unlimited; `ratio` is the group multiplier.
   * Returns the seconds to wait when limited, or 0 when allowed.
   */
  acquire(key: string, rpmLimit: number, tpmLimit: number, rpmRatio = 1, tpmRatio = 1): { retryAfter: number; tier?: 'rpm' | 'tpm' } {
    const now = Date.now();
    const rpm = effective(rpmLimit, rpmRatio);
    const tpm = effective(tpmLimit, tpmRatio);

    const t = this.current(this.tpm, key, now);
    if (tpm > 0 && t && t.count >= tpm) return { retryAfter: secondsUntil(t.resetAt, now), tier: 'tpm' };

    const r = this.current(this.rpm, key, now);
    if (rpm > 0 && r && r.count >= rpm) return { retryAfter: secondsUntil(r.resetAt, now), tier: 'rpm' };

    if (rpm > 0) this.add(this.rpm, key, 1, now);
    return { retryAfter: 0 };
  }

  /** Records tokens used by a finished request against the key's TPM window. */
  recordTokens(key: string, tokens: number): void {
    if (tokens > 0) this.add(this.tpm, key, tokens, Date.now());
  }

  private current(store: Map<string, Window>, key: string, now: number): Window | undefined {
    const w = store.get(key);
    return w && now < w.resetAt ? w : undefined;
  }

  private add(store: Map<string, Window>, key: string, delta: number, now: number): void {
    const w = this.current(store, key, now);
    if (w) w.count += delta;
    else store.set(key, { count: delta, resetAt: now + WINDOW_MS });
  }
}

function effective(limit: number, ratio: number): number {
  return limit > 0 ? Math.max(1, Math.round(limit * ratio)) : 0;
}

function secondsUntil(resetAt: number, now: number): number {
  return Math.max(1, Math.ceil((resetAt - now) / 1000));
}
