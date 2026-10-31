import { Controller, Get, Res } from '@nestjs/common';
import type { Response } from 'express';
import { RegistryService } from '../registry/registry.service.js';
import { MetricsService } from '../metrics/metrics.service.js';

/** Served on the admin port only (see main.ts). */
@Controller()
export class HealthController {
  constructor(
    private readonly registry: RegistryService,
    private readonly metrics: MetricsService,
  ) {}

  @Get('healthz')
  healthz(): Record<string, unknown> {
    return { ok: true, ...this.registry.stats() };
  }

  /** 503 until every informer has loaded its initial list, so traffic isn't sent to an empty registry. */
  @Get('readyz')
  readyz(@Res() res: Response): void {
    const ready = this.registry.isReady();
    res.status(ready ? 200 : 503).json({ ok: ready, ...this.registry.stats() });
  }

  @Get('metrics')
  async scrape(@Res() res: Response): Promise<void> {
    res.set('Content-Type', this.metrics.contentType());
    res.end(await this.metrics.registry());
  }
}
