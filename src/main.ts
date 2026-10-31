import 'reflect-metadata';

import { createServer } from 'node:http';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { NestFactory } from '@nestjs/core';
import { Logger } from '@nestjs/common';
import type { NestExpressApplication } from '@nestjs/platform-express';
import express, { type NextFunction, type Request, type Response } from 'express';

import { AppModule } from './app.module.js';
import { AppConfig } from './common/config.js';

/** Built dashboard (web/dist), next to dist/ in the image. */
const WEB_DIR = fileURLToPath(new URL('../web/dist', import.meta.url));
const ADMIN_ONLY_PATHS = /^\/(api|healthz|readyz|metrics)(\/|$)/;

/**
 * One Nest app, two listeners on the same Express instance:
 *   - proxy port (8000): tenant-facing, serves only /v1/*
 *   - admin port (8081): health, metrics, admin API and the dashboard
 */
async function bootstrap(): Promise<void> {
  const log = new Logger('main');
  // Relay handlers read the raw body themselves, so Nest must not consume the stream.
  const app = await NestFactory.create<NestExpressApplication>(AppModule, { bodyParser: false });
  const config = app.get(AppConfig);
  app.enableShutdownHooks();
  if (config.trustProxy) app.set('trust proxy', parseTrustProxy(config.trustProxy));

  const isAdminPort = (req: Request): boolean => req.socket.localPort === config.adminPort;

  app.use((req: Request, res: Response, next: NextFunction) => {
    const relayPath = req.path === '/v1' || req.path.startsWith('/v1/');
    if (relayPath === isAdminPort(req)) {
      res.status(404).json({ error: { message: 'not found', type: 'not_found' } });
      return;
    }
    next();
  });
  app.use('/api', express.json({ limit: '1mb' }));

  if (existsSync(WEB_DIR)) {
    app.use(express.static(WEB_DIR, { index: false, maxAge: '1h' }));
    // Single-page app: any other GET on the admin port returns index.html.
    app.use((req: Request, res: Response, next: NextFunction) => {
      if (req.method !== 'GET' || !isAdminPort(req) || ADMIN_ONLY_PATHS.test(req.path)) return next();
      res.sendFile('index.html', { root: WEB_DIR, headers: { 'cache-control': 'no-cache' } });
    });
  } else {
    log.warn(`dashboard not found at ${WEB_DIR}; admin API still available`);
  }

  await app.listen(config.proxyPort);
  log.log(`proxy listening on :${config.proxyPort}`);

  const admin = createServer(app.getHttpAdapter().getInstance());
  admin.listen(config.adminPort, () => log.log(`admin, health + metrics on :${config.adminPort}`));
  process.once('SIGTERM', () => admin.close());

  if (!config.adminKey) log.warn('ADMIN_KEY is not set: admin API and dashboard are disabled');
}

/** TRUST_PROXY accepts what Express does: true/false, a hop count, or a comma-separated list of CIDRs. */
function parseTrustProxy(v: string): boolean | number | string {
  if (v === 'true') return true;
  if (v === 'false') return false;
  return /^\d+$/.test(v) ? Number(v) : v;
}

void bootstrap();
