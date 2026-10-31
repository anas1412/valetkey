import { Injectable } from '@nestjs/common';
import * as prom from 'prom-client';

@Injectable()
export class MetricsService {
  private _registry: prom.Registry;

  readonly requestsTotal: prom.Counter;
  readonly tokensTotal: prom.Counter;
  readonly errorsTotal: prom.Counter;
  readonly rateLimitedTotal: prom.Counter;
  readonly keysMintedTotal: prom.Counter;
  readonly retriesTotal: prom.Counter;
  readonly channelErrors: prom.Counter;

  readonly requestDuration: prom.Histogram;
  readonly ttfb: prom.Histogram;
  readonly upstreamDuration: prom.Histogram;

  readonly channelsActive: prom.Gauge;
  readonly proxykeysActive: prom.Gauge;


  constructor() {
    this._registry = new prom.Registry();
    prom.collectDefaultMetrics({ register: this._registry });

    this.requestsTotal = new prom.Counter({
      name: 'llmproxy_requests_total',
      help: 'Total relay requests',
      labelNames: ['channel', 'model', 'group', 'proxykey_ns', 'proxykey_name', 'status_code'],
      registers: [this._registry],
    });

    this.tokensTotal = new prom.Counter({
      name: 'llmproxy_tokens_total',
      help: 'Total tokens processed',
      labelNames: ['channel', 'model', 'group', 'type'],
      registers: [this._registry],
    });

    this.errorsTotal = new prom.Counter({
      name: 'llmproxy_errors_total',
      help: 'Total relay errors',
      labelNames: ['channel', 'model', 'group', 'error_type'],
      registers: [this._registry],
    });

    this.rateLimitedTotal = new prom.Counter({
      name: 'llmproxy_rate_limited_total',
      help: 'Total rate-limited requests',
      labelNames: ['group', 'tier'],
      registers: [this._registry],
    });

    this.keysMintedTotal = new prom.Counter({
      name: 'llmproxy_keys_minted_total',
      help: 'Total virtual keys minted',
      labelNames: ['group'],
      registers: [this._registry],
    });

    this.retriesTotal = new prom.Counter({
      name: 'llmproxy_retries_total',
      help: 'Total retry attempts',
      labelNames: ['channel'],
      registers: [this._registry],
    });

    this.requestDuration = new prom.Histogram({
      name: 'llmproxy_request_duration_seconds',
      help: 'End-to-end request duration',
      labelNames: ['channel', 'model'],
      buckets: [0.1, 0.5, 1, 2, 5, 10, 30, 60, 120],
      registers: [this._registry],
    });

    this.ttfb = new prom.Histogram({
      name: 'llmproxy_ttfb_seconds',
      help: 'Time to first byte from upstream',
      labelNames: ['channel', 'model'],
      buckets: [0.05, 0.1, 0.5, 1, 2, 5, 10, 30],
      registers: [this._registry],
    });

    this.upstreamDuration = new prom.Histogram({
      name: 'llmproxy_upstream_duration_seconds',
      help: 'Upstream request duration',
      labelNames: ['channel', 'model'],
      buckets: [0.5, 1, 5, 10, 30, 60, 120, 300],
      registers: [this._registry],
    });

    this.channelsActive = new prom.Gauge({
      name: 'llmproxy_channels_active',
      help: 'Number of active channels by type',
      labelNames: ['type'],
      registers: [this._registry],
    });

    this.proxykeysActive = new prom.Gauge({
      name: 'llmproxy_proxykeys_active',
      help: 'Number of active proxy keys per group',
      labelNames: ['group'],
      registers: [this._registry],
    });

    this.channelErrors = new prom.Counter({
      name: 'llmproxy_channel_errors_total',
      help: 'Failed upstream attempts per channel (connection errors, timeouts, 429/5xx)',
      labelNames: ['channel'],
      registers: [this._registry],
    });
  }

  contentType(): string {
    return this._registry.contentType;
  }

  async registry(): Promise<string> {
    return this._registry.metrics();
  }
}
