import { Injectable, Logger, type OnModuleInit } from '@nestjs/common';
import { makeInformer, type KubernetesListObject } from '@kubernetes/client-node';
import { K8sService } from '../k8s/k8s.service.js';
import { AppConfig } from '../common/config.js';
import { RegistryService } from '../registry/registry.service.js';
import { ReconcilerService } from '../reconciler/reconciler.service.js';
import { runInformer } from './run-informer.js';
import { readyStatus, statusChanged } from '../common/status.js';
import { GROUP, VERSION, GROUP_PLURAL, type Group } from '../common/types.js';

@Injectable()
export class GroupInformerService implements OnModuleInit {
  private readonly log = new Logger(GroupInformerService.name);

  constructor(
    private readonly k8s: K8sService,
    private readonly config: AppConfig,
    private readonly registry: RegistryService,
    private readonly reconciler: ReconcilerService,
  ) {}

  async onModuleInit(): Promise<void> {
    const ns = this.config.operatorNamespace;
    const path = `/apis/${GROUP}/${VERSION}/namespaces/${ns}/${GROUP_PLURAL}`;

    const informer = makeInformer<Group>(this.k8s.kubeConfig, path, () =>
      this.k8s.listGroups(ns) as Promise<KubernetesListObject<Group>>,
    );

    const onChange = (g: Group): void => {
      this.registry.upsertGroup(g);
      void this.refreshStatus(g);
      this.reconciler.requeueAll();
    };
    informer.on('add', onChange);
    informer.on('update', onChange);
    informer.on('delete', (g: Group) => {
      this.registry.evictGroup(g.metadata!.name!);
      this.reconciler.requeueAll();
    });

    await runInformer(informer, 'groups', this.log, this.registry);
  }

  /** Recomputes every group's status — called when channels change. */
  refreshAllStatus(): void {
    for (const e of this.registry.getAllGroups()) void this.refreshStatus(e.group);
  }

  /** Ready = enabled and at least one referenced channel is enabled with a resolved key. */
  private async refreshStatus(g: Group): Promise<void> {
    const name = g.metadata!.name!;
    const missing = (g.spec.channelRefs ?? []).filter((c) => !this.registry.getChannel(c));
    const usable = this.registry.getGroupChannels(name).length;
    const enabled = (g.spec.status ?? 'Enabled') === 'Enabled';

    let reason = 'Ready';
    let message = `${usable} usable channel(s)`;
    if (!enabled) [reason, message] = ['Disabled', 'group is disabled'];
    else if (!usable) [reason, message] = ['NoChannels', 'no referenced channel is enabled with a resolved key'];
    if (missing.length) message += `; unknown channels: ${missing.join(', ')}`;

    const next = readyStatus(g.status, enabled && usable > 0, reason, message, g.metadata!.generation);
    if (!statusChanged(g.status as Record<string, unknown> | undefined, next)) return;
    await this.k8s.patchGroupStatus(this.config.operatorNamespace, name, next)
      .catch((err: unknown) => this.log.warn(`group ${name}: status patch failed: ${String(err)}`));
  }
}
