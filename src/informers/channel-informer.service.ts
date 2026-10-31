import { Injectable, Logger, type OnModuleInit } from '@nestjs/common';
import { makeInformer, type KubernetesListObject } from '@kubernetes/client-node';
import { K8sService } from '../k8s/k8s.service.js';
import { AppConfig } from '../common/config.js';
import { RegistryService } from '../registry/registry.service.js';
import { MetricsService } from '../metrics/metrics.service.js';
import { StatsService } from '../stats/stats.service.js';
import { ReconcilerService } from '../reconciler/reconciler.service.js';
import { GroupInformerService } from './group-informer.service.js';
import { runInformer } from './run-informer.js';
import { readyStatus, statusChanged } from '../common/status.js';
import { GROUP, VERSION, CHANNEL_PLURAL, type Channel } from '../common/types.js';

@Injectable()
export class ChannelInformerService implements OnModuleInit {
  private readonly log = new Logger(ChannelInformerService.name);
  /** Last reported problem per channel, so each new problem is logged once. */
  private problems = new Map<string, string | undefined>();

  constructor(
    private readonly k8s: K8sService,
    private readonly config: AppConfig,
    private readonly registry: RegistryService,
    private readonly metrics: MetricsService,
    private readonly stats: StatsService,
    private readonly reconciler: ReconcilerService,
    private readonly groups: GroupInformerService,
  ) {}

  async onModuleInit(): Promise<void> {
    const ns = this.config.operatorNamespace;
    const path = `/apis/${GROUP}/${VERSION}/namespaces/${ns}/${CHANNEL_PLURAL}`;

    const informer = makeInformer<Channel>(this.k8s.kubeConfig, path, () =>
      this.k8s.listChannels(ns) as Promise<KubernetesListObject<Channel>>,
    );

    informer.on('add', (ch: Channel) => void this.resolve(ch));
    informer.on('update', (ch: Channel) => void this.resolve(ch));
    informer.on('delete', (ch: Channel) => {
      this.registry.evictChannel(ch.metadata!.name!);
      this.problems.delete(ch.metadata!.name!);
      this.changed();
    });

    await runInformer(informer, 'channels', this.log, this.registry);
  }

  /** Reads the channel's upstream key, updates the registry and the Channel status. */
  async resolve(ch: Channel): Promise<void> {
    const name = ch.metadata!.name!;
    const ref = ch.spec.keySecretRef;
    const keyName = ref.key ?? 'apiKey';
    let upstreamKey: string | undefined;
    let problem: string | undefined;
    try {
      upstreamKey = await this.k8s.getSecretKey(this.config.operatorNamespace, ref.name, keyName);
      if (!upstreamKey) problem = `key "${keyName}" missing in secret "${ref.name}"`;
    } catch (err) {
      problem = `cannot read secret "${ref.name}": ${String(err)}`;
    }
    this.registry.upsertChannel(ch, upstreamKey);
    if (problem !== this.problems.get(name)) {
      if (problem) {
        this.log.warn(`channel ${name}: ${problem}`);
        this.stats.event('warn', 'channel-key', `channel ${name}: ${problem}`);
      } else if (this.problems.has(name)) {
        this.stats.event('info', 'channel-key', `channel ${name}: upstream key resolved`);
      }
      this.problems.set(name, problem);
    }

    const next = readyStatus(ch.status, !problem, problem ? 'SecretUnavailable' : 'KeyResolved',
      problem ?? 'upstream key resolved', ch.metadata!.generation);
    if (statusChanged(ch.status as Record<string, unknown> | undefined, next)) {
      await this.k8s.patchChannelStatus(this.config.operatorNamespace, name, next)
        .catch((err: unknown) => this.log.warn(`channel ${name}: status patch failed: ${String(err)}`));
    }
    this.changed();
  }

  private changed(): void {
    this.metrics.channelsActive.reset();
    const byType: Record<string, number> = {};
    for (const e of this.registry.getEnabledChannels()) {
      byType[e.channel.spec.type] = (byType[e.channel.spec.type] ?? 0) + 1;
    }
    for (const [type, count] of Object.entries(byType)) this.metrics.channelsActive.set({ type }, count);
    this.groups.refreshAllStatus();
    this.reconciler.requeueAll();
  }
}
