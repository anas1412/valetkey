import { Injectable, Logger, type OnModuleInit } from '@nestjs/common';
import { makeInformer, type V1Secret } from '@kubernetes/client-node';
import { K8sService } from '../k8s/k8s.service.js';
import { AppConfig } from '../common/config.js';
import { RegistryService } from '../registry/registry.service.js';
import { ChannelInformerService } from './channel-informer.service.js';
import { runInformer } from './run-informer.js';

/**
 * Watches Secrets in the operator namespace. When a Secret referenced by a
 * Channel changes or is deleted, re-resolves that channel's upstream key.
 */
@Injectable()
export class SecretInformerService implements OnModuleInit {
  private readonly log = new Logger(SecretInformerService.name);

  constructor(
    private readonly k8s: K8sService,
    private readonly config: AppConfig,
    private readonly registry: RegistryService,
    private readonly channels: ChannelInformerService,
  ) {}

  async onModuleInit(): Promise<void> {
    const ns = this.config.operatorNamespace;
    const path = `/api/v1/namespaces/${ns}/secrets`;

    const informer = makeInformer<V1Secret>(this.k8s.kubeConfig, path, () =>
      this.k8s.core.listNamespacedSecret({ namespace: ns }),
    );

    const onSecretChange = (s: V1Secret): void => {
      const name = s.metadata?.name;
      if (!name) return;
      for (const entry of this.registry.getAllChannels()) {
        if (entry.channel.spec.keySecretRef.name === name) void this.channels.resolve(entry.channel);
      }
    };

    informer.on('add', onSecretChange);
    informer.on('update', onSecretChange);
    informer.on('delete', onSecretChange);

    await runInformer(informer, 'secrets', this.log, this.registry);
  }
}
