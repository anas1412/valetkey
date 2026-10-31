import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import { createServer, type Server, type IncomingMessage } from 'node:http';
import type { AddressInfo } from 'node:net';
import express from 'express';
import { RegistryService } from '../src/registry/registry.service.js';
import { ChannelPickerService } from '../src/relay/channel-picker.service.js';
import { RateLimiterService } from '../src/relay/rate-limiter.service.js';
import { MetricsService } from '../src/metrics/metrics.service.js';
import { StatsService } from '../src/stats/stats.service.js';
import { RelayService } from '../src/relay/relay.service.js';
import { TokenAuthGuard } from '../src/auth/token-auth.guard.js';
import { buildAllowList, hashKey } from '../src/common/key-utils.js';
import type { Channel, Route } from '../src/common/types.js';

/**
 * Relay integration: a real RelayService between a real HTTP client and fake upstreams.
 * No Kubernetes — the registry is filled directly.
 */

let ok: Server, broken: Server, anthropic: Server, proxy: Server;
let proxyURL = '';
const seen: Array<{ path: string; headers: IncomingMessage['headers']; body: any }> = [];
const registry = new RegistryService();
const stats = new StatsService();
const relay = new RelayService(new ChannelPickerService(registry), new RateLimiterService(), registry, new MetricsService(), stats);

const route = (extra: Partial<Route> = {}): Route => ({
  keyHash: 'h', groupName: 'team', namespace: 'team', proxyKeyName: 'app', ...extra,
});

before(async () => {
  ok = await listen((req, body, res) => {
    seen.push({ path: req.url!, headers: req.headers, body });
    if (body.stream) {
      res.writeHead(200, { 'content-type': 'text/event-stream' });
      res.write('data: {"choices":[{"delta":{"content":"he"}}]}\n\n');
      res.end('data: {"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2}}\n\ndata: [DONE]\n\n');
      return;
    }
    res.writeHead(200, { 'content-type': 'application/json', 'set-cookie': 'upstream=secret' });
    res.end(JSON.stringify({ id: 'x', choices: [{ message: { content: 'hello' } }], usage: { prompt_tokens: 10, completion_tokens: 4 } }));
  });
  broken = await listen((_req, _body, res) => {
    res.writeHead(500, { 'content-type': 'application/json' });
    res.end('{"error":{"message":"boom"}}');
  });
  anthropic = await listen((req, body, res) => {
    seen.push({ path: req.url!, headers: req.headers, body });
    res.writeHead(200, { 'content-type': 'application/json' });
    res.end(JSON.stringify({ id: 'msg_1', model: body.model, stop_reason: 'end_turn', content: [{ type: 'text', text: 'hi from claude' }], usage: { input_tokens: 6, output_tokens: 2 } }));
  });

  registry.upsertChannel(channel('ok', 'openai', ok, 1, ['gpt-4o', 'gpt-4o-mini']), 'real-openai-key');
  registry.upsertChannel(channel('broken', 'openai', broken, 1_000_000, ['gpt-4o', 'only-broken']), 'k');
  registry.upsertChannel(channel('claude', 'anthropic', anthropic, 1, ['claude-x']), 'real-anthropic-key');
  registry.upsertGroup({ metadata: { name: 'team' }, spec: { channelRefs: ['ok', 'broken', 'claude'] } });

  const app = express();
  app.post('/v1/chat/completions', (req, res) => void relay.handle(req, res, routeFor(req), 'openai'));
  app.post('/v1/messages', (req, res) => void relay.handle(req, res, routeFor(req), 'anthropic'));
  proxy = createServer(app).listen(0);
  proxyURL = `http://127.0.0.1:${(proxy.address() as AddressInfo).port}`;
});

after(() => {
  for (const s of [ok, broken, anthropic, proxy]) s.close();
});

/** Tests pick a route variant with the x-test-route header. */
function routeFor(req: express.Request): Route {
  switch (req.headers['x-test-route']) {
    case 'mini-only': return route({ models: ['gpt-4o-mini'] });
    case 'rpm1': return route({ proxyKeyName: 'limited', rpm: 1 });
    default: return route();
  }
}

test('non-streaming request: upstream key swapped in, client headers filtered, usage recorded', async () => {
  const res = await post('/v1/chat/completions', { model: 'gpt-4o-mini', messages: [] }, {
    authorization: 'Bearer sk-proxy-tenant', cookie: 'a=b', 'accept-encoding': 'gzip', 'x-forwarded-for': '1.2.3.4',
  });
  assert.equal(res.status, 200);
  assert.equal((await res.json()).choices[0].message.content, 'hello');
  assert.equal(res.headers.get('set-cookie'), null);
  const up = seen.at(-1)!;
  assert.equal(up.path, '/v1/chat/completions');
  assert.equal(up.headers.authorization, 'Bearer real-openai-key');
  assert.equal(up.headers.cookie, undefined);
  assert.equal(up.headers['accept-encoding'], undefined);
  assert.equal(up.headers['x-forwarded-for'], undefined);
  const last = stats.recentRequests()[0];
  assert.deepEqual([last.channel, last.inputTokens, last.outputTokens, last.status], ['ok', 10, 4, 200]);
});

test('streaming request passes SSE through and counts tokens', async () => {
  const res = await post('/v1/chat/completions', { model: 'gpt-4o-mini', messages: [], stream: true });
  assert.equal(res.status, 200);
  assert.match(res.headers.get('content-type')!, /text\/event-stream/);
  const text = await res.text();
  assert.match(text, /"content":"he"/);
  assert.match(text, /\[DONE\]/);
  assert.deepEqual([stats.recentRequests()[0].inputTokens, stats.recentRequests()[0].outputTokens], [3, 2]);
});

test('a 500 from the preferred channel is retried on the next one', async () => {
  const res = await post('/v1/chat/completions', { model: 'gpt-4o', messages: [] });
  assert.equal(res.status, 200);
  assert.equal(stats.recentRequests()[0].channel, 'ok');
});

test('when every channel fails, the last upstream error is returned', async () => {
  const res = await post('/v1/chat/completions', { model: 'only-broken', messages: [] });
  assert.equal(res.status, 500);
  assert.equal((await res.json()).error.message, 'boom');
});

test('OpenAI client on an Anthropic channel gets a translated response', async () => {
  const res = await post('/v1/chat/completions', { model: 'claude-x', messages: [{ role: 'system', content: 's' }, { role: 'user', content: 'hi' }] });
  assert.equal(res.status, 200);
  const body = await res.json();
  assert.equal(body.object, 'chat.completion');
  assert.equal(body.choices[0].message.content, 'hi from claude');
  const up = seen.at(-1)!;
  assert.equal(up.path, '/v1/messages');
  assert.equal(up.headers['x-api-key'], 'real-anthropic-key');
  assert.equal(up.body.system, 's');
});

test('Anthropic client on /v1/messages is passed through natively', async () => {
  const res = await post('/v1/messages', { model: 'claude-x', system: 'native', max_tokens: 5, messages: [{ role: 'user', content: 'hi' }] });
  assert.equal(res.status, 200);
  assert.equal((await res.json()).content[0].text, 'hi from claude');
  assert.equal(seen.at(-1)!.body.system, 'native');
});

test('client errors are answered immediately with JSON', async () => {
  const noChannel = await post('/v1/chat/completions', { model: 'nobody-serves-this', messages: [] });
  assert.equal(noChannel.status, 503);
  assert.equal((await noChannel.json()).error.type, 'no_channel_available');

  const badJSON = await fetch(`${proxyURL}/v1/chat/completions`, { method: 'POST', body: '{nope', headers: { 'content-type': 'application/json' } });
  assert.equal(badJSON.status, 400);

  const anthropicShape = await post('/v1/messages', { model: 'gpt-4o', messages: [] });
  assert.equal(anthropicShape.status, 503);
  assert.equal((await anthropicShape.json()).type, 'error');
});

test('key model allowlist is enforced', async () => {
  const res = await post('/v1/chat/completions', { model: 'gpt-4o', messages: [] }, { 'x-test-route': 'mini-only' });
  assert.equal(res.status, 403);
  assert.equal((await res.json()).error.type, 'model_not_allowed');
});

test('per-key RPM limit returns 429 with retry-after', async () => {
  const first = await post('/v1/chat/completions', { model: 'gpt-4o-mini', messages: [] }, { 'x-test-route': 'rpm1' });
  assert.equal(first.status, 200);
  const second = await post('/v1/chat/completions', { model: 'gpt-4o-mini', messages: [] }, { 'x-test-route': 'rpm1' });
  assert.equal(second.status, 429);
  assert.ok(Number(second.headers.get('retry-after')) >= 1);
});

test('token guard: unknown key, wrong IP and valid key', () => {
  const reg = new RegistryService();
  reg.upsertRoute('uid', route({ keyHash: hashKey('sk-proxy-good'), allowList: buildAllowList(['10.0.0.0/8']) }));
  const guard = new TokenAuthGuard(reg);
  const ctx = (headers: Record<string, string>, ip: string) => ({
    switchToHttp: () => ({ getRequest: () => ({ headers, ip, path: '/v1/chat/completions', socket: {} }) }),
  }) as any;

  assert.throws(() => guard.canActivate(ctx({}, '10.0.0.1')), (e: any) => e.getStatus() === 401);
  assert.throws(() => guard.canActivate(ctx({ authorization: 'Bearer sk-proxy-bad' }, '10.0.0.1')), (e: any) => e.getStatus() === 401);
  assert.throws(() => guard.canActivate(ctx({ authorization: 'Bearer sk-proxy-good' }, '8.8.8.8')), (e: any) => e.getStatus() === 401);
  assert.equal(guard.canActivate(ctx({ 'x-api-key': 'sk-proxy-good' }, '::ffff:10.0.0.1')), true);
});

function channel(name: string, type: 'openai' | 'anthropic', server: Server, priority: number, models: string[]): Channel {
  const port = (server.address() as AddressInfo).port;
  return { metadata: { name }, spec: { type, baseURL: `http://127.0.0.1:${port}`, keySecretRef: { name: 's' }, priority, models } };
}

function post(path: string, body: unknown, headers: Record<string, string> = {}): Promise<Response> {
  return fetch(`${proxyURL}${path}`, {
    method: 'POST', body: JSON.stringify(body), headers: { 'content-type': 'application/json', ...headers },
  });
}

function listen(handler: (req: IncomingMessage, body: any, res: import('node:http').ServerResponse) => void): Promise<Server> {
  return new Promise((resolve) => {
    const server = createServer((req, res) => {
      const chunks: Buffer[] = [];
      req.on('data', (c: Buffer) => chunks.push(c));
      req.on('end', () => handler(req, JSON.parse(Buffer.concat(chunks).toString() || '{}'), res));
    });
    server.listen(0, () => resolve(server));
  });
}
