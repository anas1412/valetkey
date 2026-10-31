import { Injectable, Logger } from '@nestjs/common';
import type { IncomingMessage } from 'node:http';
import type { Request, Response } from 'express';
import { ChannelPickerService } from './channel-picker.service.js';
import { RateLimiterService } from './rate-limiter.service.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { StatsService } from '../stats/stats.service.js';
import { RegistryService } from '../registry/registry.service.js';
import { sendUpstream, UpstreamTimeout } from './upstream.js';
import { UsageTracker } from './usage.js';
import { ChannelAgents } from './agents.js';
import { RelayError, sendError, type InboundAPI } from './errors.js';
import { adaptOpenAI } from './adaptors/openai.adaptor.js';
import { adaptAnthropic } from './adaptors/anthropic.adaptor.js';
import type { Adaptation } from './adaptors/types.js';
import type { ChannelEntry, Route } from '../common/types.js';

const MAX_BODY = 20 * 1024 * 1024;
const MAX_ATTEMPTS = 3;
const UPSTREAM_IDLE_TIMEOUT_MS = 600_000;

/** Client headers forwarded upstream. Everything else (cookies, auth, x-forwarded-*) is dropped. */
const FORWARD_REQUEST_HEADERS = ['accept', 'user-agent', 'anthropic-version', 'anthropic-beta', 'openai-beta'];

/** Upstream headers returned to the client. */
const FORWARD_RESPONSE_HEADERS = ['content-type', 'cache-control', 'retry-after', 'x-request-id', 'request-id'];

/** Outcome of one relayed request, for metrics and the activity log. */
export interface RelayResult {
  statusCode: number;
  channel: string;
  model: string;
  inputTokens: number;
  outputTokens: number;
  latencyMs: number;
  ttfbMs: number;
  errorType?: string;
  retries: number;
}

@Injectable()
export class RelayService {
  private readonly log = new Logger(RelayService.name);
  private readonly agents = new ChannelAgents();

  constructor(
    private readonly channelPicker: ChannelPickerService,
    private readonly rateLimiter: RateLimiterService,
    private readonly registry: RegistryService,
    private readonly metrics: MetricsService,
    private readonly stats: StatsService,
  ) {}

  async handle(req: Request, res: Response, route: Route, api: InboundAPI): Promise<void> {
    const start = Date.now();
    const fail = (status: number, type: string, message: string, model = 'unknown'): void => {
      sendError(res, api, status, type, message);
      this.record(route, { statusCode: status, channel: 'none', model, inputTokens: 0, outputTokens: 0,
        latencyMs: Date.now() - start, ttfbMs: 0, errorType: type, retries: 0 });
    };

    let body: Record<string, unknown>;
    try {
      body = JSON.parse((await readBody(req, MAX_BODY)).toString('utf8'));
    } catch (err) {
      if (err instanceof BodyTooLarge) return fail(413, 'request_too_large', 'request body exceeds 20 MiB');
      return fail(400, 'invalid_request_error', 'request body must be valid JSON');
    }
    const model = body?.model;
    if (typeof model !== 'string' || !model) return fail(400, 'invalid_request_error', '"model" is required');
    if (route.models && !route.models.includes(model)) {
      return fail(403, 'model_not_allowed', `this key may not use model "${model}"`, model);
    }

    const limitKey = `${route.namespace}/${route.proxyKeyName}`;
    if (route.rpm || route.tpm) {
      const group = this.registry.getGroup(route.groupName)?.group.spec;
      const limited = this.rateLimiter.acquire(limitKey, route.rpm ?? 0, route.tpm ?? 0, group?.rpmRatio ?? 1, group?.tpmRatio ?? 1);
      if (limited.retryAfter) {
        this.metrics.rateLimitedTotal.inc({ group: route.groupName, tier: limited.tier! });
        res.setHeader('retry-after', String(limited.retryAfter));
        return fail(429, 'rate_limit_exceeded', `${limited.tier!.toUpperCase()} limit reached for this key`, model);
      }
    }

    const candidates = this.channelPicker.candidates(route.groupName, model, api).slice(0, MAX_ATTEMPTS);
    if (!candidates.length) {
      return fail(503, 'no_channel_available', `no enabled channel in group "${route.groupName}" serves model "${model}"`, model);
    }

    // Abort the in-flight upstream call if the client goes away.
    const abort = new AbortController();
    res.on('close', () => abort.abort());

    let lastStatus = 502;
    let lastType = 'upstream_error';
    for (const [attempt, entry] of candidates.entries()) {
      const channel = entry.channel.metadata!.name!;
      const isLast = attempt === candidates.length - 1;

      let adaptation: Adaptation;
      try {
        adaptation = this.adapt(entry, api, body, req);
      } catch (err) {
        if (err instanceof RelayError) return fail(err.status, err.type, err.message, model);
        throw err;
      }

      let upRes: IncomingMessage;
      const upstreamStart = Date.now();
      try {
        upRes = await sendUpstream(adaptation.request, {
          timeoutMs: UPSTREAM_IDLE_TIMEOUT_MS,
          signal: abort.signal,
          agent: this.agents.get(entry.channel, adaptation.request.url.protocol),
        });
      } catch (err) {
        if (abort.signal.aborted) return;
        this.metrics.channelErrors.inc({ channel });
        lastStatus = err instanceof UpstreamTimeout ? 504 : 502;
        lastType = err instanceof UpstreamTimeout ? 'upstream_timeout' : 'upstream_error';
        this.log.warn(`upstream ${channel} failed for ${route.namespace}/${route.proxyKeyName}: ${(err as Error).message}`);
        continue;
      }

      const status = upRes.statusCode ?? 502;
      // Retry rate limits and server errors on the next channel, before anything is sent to the client.
      if (!isLast && (status === 429 || status >= 500)) {
        upRes.resume();
        this.metrics.channelErrors.inc({ channel });
        lastStatus = status;
        lastType = status === 429 ? 'upstream_rate_limited' : 'upstream_error';
        continue;
      }

      const ttfbMs = Date.now() - start;
      const usage = await this.forward(upRes, res, adaptation);
      if (route.tpm) this.rateLimiter.recordTokens(limitKey, usage.input + usage.output);
      this.metrics.upstreamDuration.observe({ channel, model }, (Date.now() - upstreamStart) / 1000);
      if (status >= 400) this.metrics.channelErrors.inc({ channel });
      this.record(route, {
        statusCode: status, channel, model,
        inputTokens: usage.input, outputTokens: usage.output,
        latencyMs: Date.now() - start, ttfbMs,
        errorType: status >= 400 ? 'upstream_status' : undefined,
        retries: attempt,
      });
      return;
    }

    if (abort.signal.aborted) return;
    fail(lastStatus, lastType, 'all upstream channels failed', model);
  }

  private adapt(entry: ChannelEntry, api: InboundAPI, body: Record<string, unknown>, req: Request): Adaptation {
    const { spec } = entry.channel;
    const model = body.model as string;
    const upstreamBody = { ...body, model: spec.modelMapping?.[model] ?? model };
    const headers = pickHeaders(req.headers, FORWARD_REQUEST_HEADERS);
    if (spec.type === 'anthropic') {
      return adaptAnthropic(spec.baseURL, entry.upstreamKey!, upstreamBody, headers, api === 'openai');
    }
    return adaptOpenAI(spec.baseURL, entry.upstreamKey!, upstreamBody, headers);
  }

  /** Streams (or buffers and translates) the upstream response to the client. Resolves with token usage. */
  private forward(upRes: IncomingMessage, res: Response, adaptation: Adaptation): Promise<UsageTracker> {
    const usage = new UsageTracker();
    const status = upRes.statusCode ?? 502;
    const isStream = upRes.headers['content-type']?.includes('text/event-stream') ?? false;
    const headers = pickHeaders(upRes.headers, FORWARD_RESPONSE_HEADERS);

    return new Promise((resolve) => {
      upRes.on('error', () => {
        res.destroy();
        resolve(usage);
      });

      if (isStream) {
        const translator = adaptation.createStreamTranslator?.();
        res.writeHead(status, { ...headers, 'content-type': 'text/event-stream', 'cache-control': 'no-cache' });
        upRes.setEncoding('utf8');
        upRes.on('data', (chunk: string) => {
          usage.feedSSE(chunk);
          res.write(translator ? translator.push(chunk) : chunk);
        });
        upRes.on('end', () => {
          usage.endSSE();
          res.end(translator?.end());
          resolve(usage);
        });
        return;
      }

      const chunks: Buffer[] = [];
      upRes.on('data', (chunk: Buffer) => chunks.push(chunk));
      upRes.on('end', () => {
        const raw = Buffer.concat(chunks);
        usage.feedJSON(raw.toString('utf8'));
        let out = raw;
        if (adaptation.translateJSON) {
          try {
            out = Buffer.from(JSON.stringify(adaptation.translateJSON(JSON.parse(raw.toString('utf8')), status)));
            headers['content-type'] = 'application/json';
          } catch {
            // Not JSON (e.g. an HTML error page from a gateway) — pass it through untouched.
          }
        }
        res.writeHead(status, { ...headers, 'content-length': String(out.length) });
        res.end(out);
        resolve(usage);
      });
    });
  }

  private record(route: Route, result: RelayResult): void {
    const labels = { channel: result.channel, model: result.model, group: route.groupName };
    this.metrics.requestsTotal.inc({
      ...labels, proxykey_ns: route.namespace, proxykey_name: route.proxyKeyName, status_code: String(result.statusCode),
    });
    if (result.inputTokens > 0) this.metrics.tokensTotal.inc({ ...labels, type: 'input' }, result.inputTokens);
    if (result.outputTokens > 0) this.metrics.tokensTotal.inc({ ...labels, type: 'output' }, result.outputTokens);
    this.metrics.requestDuration.observe({ channel: result.channel, model: result.model }, result.latencyMs / 1000);
    if (result.ttfbMs > 0) this.metrics.ttfb.observe({ channel: result.channel, model: result.model }, result.ttfbMs / 1000);
    if (result.errorType) this.metrics.errorsTotal.inc({ ...labels, error_type: result.errorType });
    if (result.retries > 0) this.metrics.retriesTotal.inc({ channel: result.channel }, result.retries);
    this.stats.recordRequest({
      time: Date.now(), namespace: route.namespace, key: route.proxyKeyName, group: route.groupName,
      channel: result.channel, model: result.model, status: result.statusCode,
      inputTokens: result.inputTokens, outputTokens: result.outputTokens, latencyMs: result.latencyMs,
      error: result.errorType,
    });
  }
}

class BodyTooLarge extends Error {}

function readBody(req: Request, limit: number): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    const chunks: Buffer[] = [];
    let size = 0;
    req.on('data', (chunk: Buffer) => {
      size += chunk.length;
      if (size > limit) {
        req.pause();
        reject(new BodyTooLarge());
        return;
      }
      chunks.push(chunk);
    });
    req.on('end', () => resolve(Buffer.concat(chunks)));
    req.on('error', reject);
  });
}

function pickHeaders(headers: Record<string, string | string[] | undefined>, names: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const name of names) {
    const v = headers[name];
    if (v !== undefined) out[name] = Array.isArray(v) ? v.join(', ') : v;
  }
  return out;
}
