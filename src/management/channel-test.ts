import { request as httpsRequest } from 'node:https';
import { request as httpRequest } from 'node:http';
import { joinURL } from '../relay/upstream.js';
import { ChannelAgents } from '../relay/agents.js';
import type { ChannelEntry } from '../common/types.js';

const agents = new ChannelAgents();
const TIMEOUT_MS = 10_000;

export interface ChannelTestResult {
  success: boolean;
  status?: number;
  latencyMs: number;
  message: string;
}

/** Calls the provider's model list endpoint with the channel's real key. Sends no prompt. */
export function testChannel(entry: ChannelEntry): Promise<ChannelTestResult> {
  const { spec } = entry.channel;
  if (!entry.upstreamKey) {
    return Promise.resolve({ success: false, latencyMs: 0, message: 'no API key resolved from the channel Secret' });
  }
  const url = joinURL(spec.baseURL, '/v1/models');
  const headers: Record<string, string> = spec.type === 'anthropic'
    ? { 'x-api-key': entry.upstreamKey, 'anthropic-version': '2023-06-01' }
    : { authorization: `Bearer ${entry.upstreamKey}` };
  const start = Date.now();

  return new Promise((resolve) => {
    const doRequest = url.protocol === 'https:' ? httpsRequest : httpRequest;
    const req = doRequest(url, {
      method: 'GET', headers, timeout: TIMEOUT_MS, agent: agents.get(entry.channel, url.protocol),
    }, (res) => {
      res.resume();
      const status = res.statusCode ?? 0;
      const ok = status >= 200 && status < 300;
      resolve({
        success: ok, status, latencyMs: Date.now() - start,
        message: ok ? `reachable, key accepted (HTTP ${status})`
          : status === 401 || status === 403 ? `provider rejected the API key (HTTP ${status})`
          : `provider answered HTTP ${status}`,
      });
    });
    req.on('timeout', () => req.destroy(new Error(`no answer within ${TIMEOUT_MS / 1000}s`)));
    req.on('error', (err) => resolve({ success: false, latencyMs: Date.now() - start, message: err.message }));
    req.end();
  });
}
