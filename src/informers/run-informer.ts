import type { Logger } from '@nestjs/common';
import type { Informer, KubernetesObject } from '@kubernetes/client-node';
import type { RegistryService } from '../registry/registry.service.js';

const RESTART_MS = 5_000;

/**
 * Starts an informer, marks it synced once the initial list is loaded, and
 * restarts it after errors — at most one pending restart at a time.
 */
export async function runInformer<T extends KubernetesObject>(
  informer: Informer<T>,
  name: string,
  log: Logger,
  registry: RegistryService,
): Promise<void> {
  let pending = false;
  const scheduleRestart = (): void => {
    if (pending) return;
    pending = true;
    setTimeout(() => {
      pending = false;
      informer.start().catch((err: unknown) => {
        log.error(`${name} informer restart failed: ${String(err)}`);
        scheduleRestart();
      });
    }, RESTART_MS);
  };

  informer.on('error', (err: unknown) => {
    log.error(`${name} informer error, restarting in ${RESTART_MS / 1000}s: ${String(err)}`);
    scheduleRestart();
  });

  await informer.start();
  registry.markSynced(name);
  log.log(`${name} informer started`);
}
