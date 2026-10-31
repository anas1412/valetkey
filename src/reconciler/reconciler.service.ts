import { Injectable, Logger, type OnModuleDestroy, type OnModuleInit } from '@nestjs/common';
import type { V1Secret } from '@kubernetes/client-node';
import { K8sService } from '../k8s/k8s.service.js';
import { AppConfig } from '../common/config.js';
import { RegistryService } from '../registry/registry.service.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { StatsService } from '../stats/stats.service.js';
import { buildAllowList, generateVirtualKey, hashKey } from '../common/key-utils.js';
import { readyStatus, statusChanged } from '../common/status.js';
import { GROUP, VERSION, type ProxyKey } from '../common/types.js';

const MANAGED_BY = 'llmproxy-operator';
const RESYNC_MS = 10 * 60_000;
const RETRY_MS = 5_000;

/** Tenant Secret name for a ProxyKey */
export function secretNameFor(proxyKeyName: string): string {
  return `${proxyKeyName}-llmproxy`;
}

@Injectable()
export class ReconcilerService implements OnModuleInit, OnModuleDestroy {
  private readonly log = new Logger(ReconcilerService.name);
  private queue = new Map<string, ProxyKey>();
  /** Latest copy of every ProxyKey seen, so Group/Channel changes can requeue them. */
  private known = new Map<string, ProxyKey>();
  private running = false;
  private resync?: NodeJS.Timeout;

  constructor(
    private readonly k8s: K8sService,
    private readonly config: AppConfig,
    private readonly registry: RegistryService,
    private readonly metrics: MetricsService,
    private readonly stats: StatsService,
  ) {}

  onModuleInit(): void {
    this.resync = setInterval(() => this.requeueAll(), RESYNC_MS);
  }

  onModuleDestroy(): void {
    clearInterval(this.resync);
  }

  enqueue(pk: ProxyKey): void {
    const id = `${pk.metadata!.namespace}/${pk.metadata!.name}`;
    this.known.set(id, pk);
    this.queue.set(id, pk);
    void this.drain();
  }

  /** Re-reconcile every known ProxyKey (after a Group or Channel change, and periodically). */
  requeueAll(): void {
    for (const pk of this.known.values()) this.queue.set(`${pk.metadata!.namespace}/${pk.metadata!.name}`, pk);
    void this.drain();
  }

  handleDelete(pk: ProxyKey): void {
    const id = `${pk.metadata!.namespace}/${pk.metadata!.name}`;
    this.registry.evictRouteByUid(pk.metadata!.uid!);
    this.queue.delete(id);
    this.known.delete(id);
    this.refreshProxyKeyGauge();
  }

  /** All ProxyKeys currently known, including not-ready and disabled ones. */
  getAll(): ProxyKey[] {
    return [...this.known.values()];
  }

  get(namespace: string, name: string): ProxyKey | undefined {
    return this.known.get(`${namespace}/${name}`);
  }

  private async drain(): Promise<void> {
    if (this.running) return;
    this.running = true;
    try {
      while (this.queue.size > 0) {
        const [id, pk] = this.queue.entries().next().value as [string, ProxyKey];
        this.queue.delete(id);
        try {
          await this.reconcile(pk);
        } catch (err) {
          this.log.error(`reconcile ${id} failed: ${String(err)}`);
          this.stats.event('error', 'reconcile', `${id}: ${String(err)}`);
          await this.setStatus(pk, false, 'ReconcileError', String(err)).catch(() => {});
          setTimeout(() => {
            const latest = this.known.get(id);
            if (latest) this.enqueue(latest);
          }, RETRY_MS);
        }
      }
    } finally {
      this.running = false;
    }
  }

  private async reconcile(pk: ProxyKey): Promise<void> {
    const ns = pk.metadata!.namespace!;
    const name = pk.metadata!.name!;
    const uid = pk.metadata!.uid!;
    const secretName = secretNameFor(name);
    // Group is always the ProxyKey's namespace — tenants can't pick another one.
    const groupRef = ns;

    const notReady = async (reason: string, message: string): Promise<void> => {
      this.registry.evictRouteByUid(uid);
      this.refreshProxyKeyGauge();
      await this.setStatus(pk, false, reason, message);
    };

    if ((pk.spec.status ?? 'Enabled') !== 'Enabled') return notReady('Disabled', 'key is disabled');

    const groupEntry = this.registry.getGroup(groupRef);
    if (!groupEntry) return notReady('GroupNotFound', `group "${groupRef}" not found`);
    if ((groupEntry.group.spec.status ?? 'Enabled') !== 'Enabled') {
      return notReady('GroupDisabled', `group "${groupRef}" is disabled`);
    }
    const groupChannels = this.registry.getGroupChannels(groupRef);
    if (groupChannels.length === 0) {
      return notReady('NoChannels', `group "${groupRef}" has no enabled channels with a resolved key`);
    }

    let allowList;
    if (pk.spec.subnets?.length) {
      try {
        allowList = buildAllowList(pk.spec.subnets);
      } catch (err) {
        return notReady('InvalidSubnets', (err as Error).message);
      }
    }

    const primaryChannel = groupChannels[0].channel.metadata!.name!;

    // Mint a virtual key unless the Secret already holds the one recorded in status.
    let keyHash = pk.status?.keyHash;
    const secret = await this.k8s.readSecret(ns, secretName);
    const secretKeyB64 = secret?.data?.LLMPROXY_KEY;
    const secretHash = secretKeyB64 ? hashKey(Buffer.from(secretKeyB64, 'base64').toString('utf8')) : undefined;

    if (!secretHash || (keyHash && secretHash !== keyHash)) {
      const virtualKey = generateVirtualKey();
      keyHash = hashKey(virtualKey);
      await this.k8s.createOrReplaceSecret(ns, secretName, this.buildSecret(pk, secretName, virtualKey, primaryChannel, groupRef));
      this.log.log(`minted key for ${ns}/${name} -> secret ${secretName} (group: ${groupRef})`);
      this.stats.event('info', 'key-minted', `new key for ${ns}/${name} (secret ${secretName})`);
      this.metrics.keysMintedTotal.inc({ group: groupRef });
    } else if (!keyHash) {
      keyHash = secretHash;
    }

    this.registry.upsertRoute(uid, {
      keyHash,
      groupName: groupRef,
      namespace: ns,
      proxyKeyName: name,
      models: pk.spec.models?.length ? pk.spec.models : undefined,
      allowList,
      rpm: pk.spec.rpm || undefined,
      tpm: pk.spec.tpm || undefined,
    });
    this.refreshProxyKeyGauge();
    await this.setStatus(pk, true, 'Active', 'Active', { keyHash, secretName });
  }

  private refreshProxyKeyGauge(): void {
    const byGroup: Record<string, number> = {};
    for (const g of this.registry.getAllGroups()) byGroup[g.group.metadata!.name!] = 0;
    for (const r of this.registry.getAllRoutes()) byGroup[r.groupName] = (byGroup[r.groupName] ?? 0) + 1;
    this.metrics.proxykeysActive.reset();
    for (const [group, count] of Object.entries(byGroup)) this.metrics.proxykeysActive.set({ group }, count);
  }

  private buildSecret(pk: ProxyKey, secretName: string, virtualKey: string, channel: string, groupRef: string): V1Secret {
    return {
      metadata: {
        name: secretName,
        namespace: pk.metadata!.namespace!,
        labels: { 'app.kubernetes.io/managed-by': MANAGED_BY },
        ownerReferences: [{
          apiVersion: `${GROUP}/${VERSION}`,
          kind: 'ProxyKey',
          name: pk.metadata!.name!,
          uid: pk.metadata!.uid!,
          controller: true,
          blockOwnerDeletion: false,
        }],
      },
      type: 'Opaque',
      stringData: {
        LLMPROXY_KEY: virtualKey,
        LLMPROXY_ENDPOINT: this.config.proxyURL,
        LLMPROXY_CHANNEL: channel,
        LLMPROXY_GROUP: groupRef,
      },
    };
  }

  /** Patches status only when something changed, so status updates don't loop through the informer. */
  private async setStatus(
    pk: ProxyKey, ready: boolean, reason: string, message: string,
    extra: { keyHash?: string; secretName?: string } = {},
  ): Promise<void> {
    const next = {
      ...readyStatus(pk.status, ready, reason, message, pk.metadata!.generation),
      keyHash: extra.keyHash ?? pk.status?.keyHash,
      secretName: extra.secretName ?? pk.status?.secretName,
    };
    if (!statusChanged(pk.status as Record<string, unknown> | undefined, next)) return;
    await this.k8s.patchProxyKeyStatus(pk.metadata!.namespace!, pk.metadata!.name!, next);
  }
}
