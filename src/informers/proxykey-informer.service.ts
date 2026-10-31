import { Injectable, Logger, type OnModuleInit } from '@nestjs/common';
import { makeInformer, type KubernetesListObject } from '@kubernetes/client-node';
import { K8sService } from '../k8s/k8s.service.js';
import { RegistryService } from '../registry/registry.service.js';
import { ReconcilerService } from '../reconciler/reconciler.service.js';
import { runInformer } from './run-informer.js';
import { GROUP, VERSION, PROXYKEY_PLURAL, type ProxyKey } from '../common/types.js';

@Injectable()
export class ProxyKeyInformerService implements OnModuleInit {
  private readonly log = new Logger(ProxyKeyInformerService.name);

  constructor(
    private readonly k8s: K8sService,
    private readonly registry: RegistryService,
    private readonly reconciler: ReconcilerService,
  ) {}

  async onModuleInit(): Promise<void> {
    const path = `/apis/${GROUP}/${VERSION}/${PROXYKEY_PLURAL}`;

    const informer = makeInformer<ProxyKey>(this.k8s.kubeConfig, path, () =>
      this.k8s.listProxyKeys() as Promise<KubernetesListObject<ProxyKey>>,
    );

    informer.on('add', (pk: ProxyKey) => this.reconciler.enqueue(pk));
    informer.on('update', (pk: ProxyKey) => this.reconciler.enqueue(pk));
    informer.on('delete', (pk: ProxyKey) => this.reconciler.handleDelete(pk));

    await runInformer(informer, 'proxykeys', this.log, this.registry);
  }
}
