import { Agent as HttpAgent } from 'node:http';
import { Agent as HttpsAgent } from 'node:https';
import type { Channel } from '../common/types.js';

/**
 * Per-channel HTTP agents for `skipTLSVerify` and `proxyURL`. Channels that set
 * neither use Node's global agent. Agents are rebuilt when the channel changes.
 */
export class ChannelAgents {
  private cache = new Map<string, { version: string; agent: HttpAgent }>();

  get(channel: Channel, protocol: string): HttpAgent | undefined {
    const { skipTLSVerify, proxyURL } = channel.spec;
    if (!skipTLSVerify && !proxyURL) return undefined;

    const key = `${channel.metadata!.name}|${protocol}`;
    const version = channel.metadata!.resourceVersion ?? '';
    const hit = this.cache.get(key);
    if (hit?.version === version) return hit.agent;

    hit?.agent.destroy();
    // proxyEnv makes the agent tunnel through the given proxy (Node ≥ 24.5).
    const proxyEnv = proxyURL ? { HTTP_PROXY: proxyURL, HTTPS_PROXY: proxyURL } : undefined;
    const agent = protocol === 'https:'
      ? new HttpsAgent({ keepAlive: true, rejectUnauthorized: !skipTLSVerify, proxyEnv })
      : new HttpAgent({ keepAlive: true, proxyEnv });
    this.cache.set(key, { version, agent });
    return agent;
  }
}
