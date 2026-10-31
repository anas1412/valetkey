import { Body, Controller, Delete, Get, HttpCode, HttpException, Param, Post, Put, UseGuards } from '@nestjs/common';
import { K8sService } from '../k8s/k8s.service.js';
import { ReconcilerService, secretNameFor } from '../reconciler/reconciler.service.js';
import { StatsService } from '../stats/stats.service.js';
import { AdminAuthGuard } from '../auth/admin-auth.guard.js';
import { k8sCall, assertName } from './k8s-errors.js';
import { PROXYKEY_PLURAL, type ProxyKey, type ProxyKeySpec } from '../common/types.js';

export interface ProxyKeyView {
  namespace: string;
  name: string;
  spec: ProxyKeySpec;
  ready: boolean;
  message?: string;
  secretName?: string;
  requests: number;
  lastUsed?: number;
}

@Controller('api/v1/proxykeys')
@UseGuards(AdminAuthGuard)
export class ProxyKeysController {
  constructor(
    private readonly k8s: K8sService,
    private readonly reconciler: ReconcilerService,
    private readonly stats: StatsService,
  ) {}

  @Get()
  list(): ProxyKeyView[] {
    return this.reconciler.getAll().map((pk) => this.view(pk));
  }

  @Get(':ns/:name')
  get(@Param('ns') ns: string, @Param('name') name: string): ProxyKeyView {
    return this.view(this.find(ns, name));
  }

  @Post()
  async create(@Body() body: { namespace?: string; name?: string; spec?: ProxyKeySpec }): Promise<ProxyKeyView> {
    assertName(body?.namespace, 'tenant');
    assertName(body?.name, 'key name');
    const { namespace, name } = body;
    const created = await k8sCall(() => this.k8s.createObject<ProxyKey>(PROXYKEY_PLURAL, namespace, {
      apiVersion: 'llmproxy.llmproxy.io/v1alpha1', kind: 'ProxyKey', metadata: { name, namespace }, spec: body.spec ?? {},
    }));
    return this.view(created);
  }

  @Put(':ns/:name')
  async update(@Param('ns') ns: string, @Param('name') name: string, @Body() body: { spec?: ProxyKeySpec }): Promise<ProxyKeyView> {
    if (!body?.spec) throw new HttpException({ message: '"spec" is required' }, 400);
    return this.view(await k8sCall(() => this.k8s.replaceSpec<ProxyKey>(PROXYKEY_PLURAL, ns, name, body.spec!)));
  }

  /** Reveals the virtual key from the tenant Secret. */
  @Get(':ns/:name/key')
  async key(@Param('ns') ns: string, @Param('name') name: string): Promise<{ key: string }> {
    this.find(ns, name);
    const key = await k8sCall(() => this.k8s.getSecretKey(ns, secretNameFor(name), 'LLMPROXY_KEY'));
    if (!key) throw new HttpException({ message: 'key not minted yet — check the key status' }, 404);
    return { key };
  }

  /** Issues a new virtual key: the old Secret is deleted and the reconciler mints a fresh one. The old key stops working. */
  @Post(':ns/:name/rotate')
  @HttpCode(204)
  async rotate(@Param('ns') ns: string, @Param('name') name: string): Promise<void> {
    const pk = this.find(ns, name);
    await k8sCall(() => this.k8s.deleteSecret(ns, secretNameFor(name)));
    this.reconciler.enqueue(pk);
  }

  @Delete(':ns/:name')
  @HttpCode(204)
  async delete(@Param('ns') ns: string, @Param('name') name: string): Promise<void> {
    await k8sCall(() => this.k8s.deleteObject(PROXYKEY_PLURAL, ns, name));
  }

  private find(ns: string, name: string): ProxyKey {
    const pk = this.reconciler.get(ns, name);
    if (!pk) throw new HttpException({ message: `key "${ns}/${name}" not found` }, 404);
    return pk;
  }

  private view(pk: ProxyKey): ProxyKeyView {
    const namespace = pk.metadata!.namespace!;
    const name = pk.metadata!.name!;
    return {
      namespace,
      name,
      spec: pk.spec ?? {},
      ready: pk.status?.ready ?? false,
      message: pk.status?.message,
      secretName: pk.status?.secretName,
      ...this.stats.keyUsage(namespace, name),
    };
  }
}
