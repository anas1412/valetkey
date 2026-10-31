import { Injectable, type OnModuleInit } from '@nestjs/common';
import {
  KubeConfig,
  CoreV1Api,
  CustomObjectsApi,
  Watch,
  PatchStrategy,
  setHeaderOptions,
  type KubernetesListObject,
  type V1Secret,
} from '@kubernetes/client-node';
import {
  GROUP,
  VERSION,
  CHANNEL_PLURAL,
  PROXYKEY_PLURAL,
  GROUP_PLURAL,
  type Channel,
  type ProxyKey,
  type Group,
} from '../common/types.js';

/** True when a client-node call failed with HTTP 404. */
export function isNotFound(err: unknown): boolean {
  return (err as { code?: number } | null)?.code === 404;
}

@Injectable()
export class K8sService implements OnModuleInit {
  private kc!: KubeConfig;
  private _core!: CoreV1Api;
  private _custom!: CustomObjectsApi;
  private _watch!: Watch;

  onModuleInit(): void {
    this.kc = new KubeConfig();
    try {
      this.kc.loadFromCluster();
    } catch {
      this.kc.loadFromDefault();
    }
    this._core = this.kc.makeApiClient(CoreV1Api);
    this._custom = this.kc.makeApiClient(CustomObjectsApi);
    this._watch = new Watch(this.kc);
  }

  get core(): CoreV1Api { return this._core; }
  get custom(): CustomObjectsApi { return this._custom; }
  get watch(): Watch { return this._watch; }
  get kubeConfig(): KubeConfig { return this.kc; }

  listChannels(ns: string): Promise<KubernetesListObject<Channel>> {
    return this._custom.listNamespacedCustomObject({
      group: GROUP, version: VERSION, namespace: ns, plural: CHANNEL_PLURAL,
    }) as Promise<KubernetesListObject<Channel>>;
  }

  listProxyKeys(): Promise<KubernetesListObject<ProxyKey>> {
    return this._custom.listClusterCustomObject({
      group: GROUP, version: VERSION, plural: PROXYKEY_PLURAL,
    }) as Promise<KubernetesListObject<ProxyKey>>;
  }

  listGroups(ns: string): Promise<KubernetesListObject<Group>> {
    return this._custom.listNamespacedCustomObject({
      group: GROUP, version: VERSION, namespace: ns, plural: GROUP_PLURAL,
    }) as Promise<KubernetesListObject<Group>>;
  }

  async patchGroupStatus(ns: string, name: string, status: NonNullable<Group['status']>): Promise<void> {
    await this._custom.patchNamespacedCustomObjectStatus(
      { group: GROUP, version: VERSION, namespace: ns, plural: GROUP_PLURAL, name, body: { status } },
      setHeaderOptions('Content-Type', PatchStrategy.MergePatch),
    ) as unknown as Promise<void>;
  }

  async patchChannelStatus(ns: string, name: string, status: NonNullable<Channel['status']>): Promise<void> {
    await this._custom.patchNamespacedCustomObjectStatus(
      { group: GROUP, version: VERSION, namespace: ns, plural: CHANNEL_PLURAL, name, body: { status } },
      setHeaderOptions('Content-Type', PatchStrategy.MergePatch),
    ) as unknown as Promise<void>;
  }

  async patchProxyKeyStatus(ns: string, name: string, status: NonNullable<ProxyKey['status']>): Promise<void> {
    await this._custom.patchNamespacedCustomObjectStatus(
      { group: GROUP, version: VERSION, namespace: ns, plural: PROXYKEY_PLURAL, name, body: { status } },
      setHeaderOptions('Content-Type', PatchStrategy.MergePatch),
    ) as unknown as Promise<void>;
  }

  /** Returns the Secret, or undefined if it does not exist. Other API errors are thrown. */
  async readSecret(ns: string, name: string): Promise<V1Secret | undefined> {
    try {
      return await this._core.readNamespacedSecret({ name, namespace: ns });
    } catch (err) {
      if (isNotFound(err)) return undefined;
      throw err;
    }
  }

  async getSecretKey(ns: string, name: string, key: string): Promise<string | undefined> {
    const secret = await this.readSecret(ns, name);
    const b64 = secret?.data?.[key];
    return b64 ? Buffer.from(b64, 'base64').toString('utf8') : undefined;
  }

  getObject<T>(plural: string, ns: string, name: string): Promise<T> {
    return this._custom.getNamespacedCustomObject({ group: GROUP, version: VERSION, namespace: ns, plural, name }) as Promise<T>;
  }

  createObject<T>(plural: string, ns: string, body: T): Promise<T> {
    return this._custom.createNamespacedCustomObject({
      group: GROUP, version: VERSION, namespace: ns, plural, body: body as object,
    }) as Promise<T>;
  }

  /** Replaces only `spec`, keeping metadata and status of the live object. */
  async replaceSpec<T extends { spec: unknown }>(plural: string, ns: string, name: string, spec: T['spec']): Promise<T> {
    const current = await this.getObject<T>(plural, ns, name);
    return this._custom.replaceNamespacedCustomObject({
      group: GROUP, version: VERSION, namespace: ns, plural, name, body: { ...current, spec } as object,
    }) as Promise<T>;
  }

  async deleteObject(plural: string, ns: string, name: string): Promise<void> {
    await this._custom.deleteNamespacedCustomObject({ group: GROUP, version: VERSION, namespace: ns, plural, name });
  }

  /** Deletes a Secret; a missing Secret is not an error. */
  async deleteSecret(ns: string, name: string): Promise<void> {
    try {
      await this._core.deleteNamespacedSecret({ name, namespace: ns });
    } catch (err) {
      if (!isNotFound(err)) throw err;
    }
  }

  async listNamespaces(): Promise<string[]> {
    const list = await this._core.listNamespace();
    return list.items.map((n) => n.metadata!.name!);
  }

  async createOrReplaceSecret(ns: string, name: string, body: V1Secret): Promise<void> {
    try {
      await this._core.replaceNamespacedSecret({ name, namespace: ns, body });
    } catch (err) {
      if (!isNotFound(err)) throw err;
      await this._core.createNamespacedSecret({ namespace: ns, body });
    }
  }
}
