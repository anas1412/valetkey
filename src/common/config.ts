import { Injectable } from '@nestjs/common';

@Injectable()
export class AppConfig {
  /** Namespace where Channel/Group CRDs and provider Secrets live */
  readonly operatorNamespace: string;

  /** URL tenants use to reach this proxy (injected into tenant Secrets) */
  readonly proxyURL: string;

  /** Tenant-facing port: only /v1/* is served here */
  readonly proxyPort: number;

  /** Internal port: health, metrics, admin API and dashboard */
  readonly adminPort: number;

  /** Admin key for the management API and dashboard. Unset = admin API disabled. */
  readonly adminKey: string | undefined;

  /** Express "trust proxy" setting, so req.ip is the real client behind an ingress. */
  readonly trustProxy: string | undefined;

  constructor() {
    this.operatorNamespace = process.env.OPERATOR_NAMESPACE ?? 'llmproxy-system';
    this.proxyURL = process.env.PROXY_URL ?? 'http://llmproxy.llmproxy-system.svc.cluster.local:8000';
    this.proxyPort = Number(process.env.PROXY_PORT ?? 8000);
    this.adminPort = Number(process.env.ADMIN_PORT ?? process.env.HEALTH_PORT ?? 8081);
    this.adminKey = process.env.ADMIN_KEY || undefined;
    this.trustProxy = process.env.TRUST_PROXY || undefined;
  }
}
