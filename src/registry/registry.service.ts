import { Injectable } from '@nestjs/common';
import type { Channel, Group } from '../common/types.js';
import type { Route, ChannelEntry, GroupEntry } from '../common/types.js';

/**
 * In-memory routing table built entirely from informer events.
 * No persistence — rebuilt on restart via informer initial lists.
 *
 * Three maps:
 *   keyHash → Route        (for auth lookups on every request)
 *   channelName → ChannelEntry  (for channel metadata + resolved upstream key)
 *   groupName → GroupEntry (for tenant definitions)
 */
@Injectable()
export class RegistryService {
  private routes = new Map<string, Route>();
  private byUid = new Map<string, string>(); // ProxyKey uid → keyHash
  private channels = new Map<string, ChannelEntry>();
  private groups = new Map<string, GroupEntry>();

  // -- Route management (ProxyKey lifecycle) ---------------------------------

  upsertRoute(uid: string, route: Route): void {
    const old = this.byUid.get(uid);
    if (old && old !== route.keyHash) this.routes.delete(old);
    this.byUid.set(uid, route.keyHash);
    this.routes.set(route.keyHash, route);
  }

  evictRouteByUid(uid: string): void {
    const hash = this.byUid.get(uid);
    if (hash) {
      this.routes.delete(hash);
      this.byUid.delete(uid);
    }
  }

  /** All active routes */
  getAllRoutes(): Route[] {
    return [...this.routes.values()];
  }

  /** Look up a route by hashed virtual key — called on every request */
  lookup(keyHash: string): Route | undefined {
    return this.routes.get(keyHash);
  }

  // -- Channel management ----------------------------------------------------

  upsertChannel(channel: Channel, upstreamKey?: string): void {
    this.channels.set(channel.metadata!.name!, { channel, upstreamKey });
  }

  evictChannel(name: string): void {
    this.channels.delete(name);
  }

  getChannel(name: string): ChannelEntry | undefined {
    return this.channels.get(name);
  }

  /** All channels with resolved keys — for channel picker */
  getAllChannels(): ChannelEntry[] {
    return [...this.channels.values()];
  }

  /** All enabled channels with resolved keys */
  getEnabledChannels(): ChannelEntry[] {
    return this.getAllChannels().filter(
      (e) => (e.channel.spec.status ?? 'Enabled') === 'Enabled' && e.upstreamKey,
    );
  }

  // -- Group management -------------------------------------------------------

  upsertGroup(group: Group): void {
    this.groups.set(group.metadata!.name!, { group });
  }

  evictGroup(name: string): void {
    this.groups.delete(name);
  }

  getGroup(name: string): GroupEntry | undefined {
    return this.groups.get(name);
  }

  /** All groups in the registry */
  getAllGroups(): GroupEntry[] {
    return [...this.groups.values()];
  }

  /** Returns enabled ChannelEntries that belong to the group's channelRefs */
  getGroupChannels(groupName: string): ChannelEntry[] {
    const group = this.groups.get(groupName);
    if (!group) return [];
    const refs = group.group.spec.channelRefs ?? [];
    return refs
      .map((name) => this.channels.get(name))
      .filter((e): e is ChannelEntry => !!e && (e.channel.spec.status ?? 'Enabled') === 'Enabled' && !!e.upstreamKey);
  }

  // -- Readiness ---------------------------------------------------------------

  private synced = new Set<string>();
  private static readonly INFORMERS = ['channels', 'groups', 'secrets', 'proxykeys'];

  /** Called by each informer once its initial list has been loaded. */
  markSynced(informer: string): void {
    this.synced.add(informer);
  }

  /** True once every informer has completed its initial list. */
  isReady(): boolean {
    return RegistryService.INFORMERS.every((i) => this.synced.has(i));
  }

  // -- Stats for health/metrics -----------------------------------------------

  stats(): { routes: number; channels: number; groups: number } {
    return { routes: this.routes.size, channels: this.channels.size, groups: this.groups.size };
  }
}
