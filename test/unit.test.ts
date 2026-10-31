import { test } from 'node:test';
import assert from 'node:assert/strict';
import { buildAllowList, ipAllowed, secretsEqual, generateVirtualKey, hashKey } from '../src/common/key-utils.js';
import { UsageTracker } from '../src/relay/usage.js';
import { joinURL } from '../src/relay/upstream.js';
import { RateLimiterService } from '../src/relay/rate-limiter.service.js';
import { ChannelPickerService, servesModel } from '../src/relay/channel-picker.service.js';
import { RegistryService } from '../src/registry/registry.service.js';
import { readyStatus, statusChanged } from '../src/common/status.js';
import type { Channel, Group } from '../src/common/types.js';

test('virtual keys are prefixed, unique, and hash deterministically', () => {
  const a = generateVirtualKey();
  assert.match(a, /^sk-proxy-[A-Za-z0-9_-]{32}$/);
  assert.notEqual(a, generateVirtualKey());
  assert.equal(hashKey(a), hashKey(a));
  assert.ok(secretsEqual('abc', 'abc'));
  assert.ok(!secretsEqual('abc', 'abcd'));
});

test('IP allow list matches IPv4, IPv6, mapped IPv4 and single hosts', () => {
  const list = buildAllowList(['10.0.0.0/8', '2001:db8::/32', '192.168.1.5']);
  assert.ok(ipAllowed('10.1.2.3', list));
  assert.ok(ipAllowed('::ffff:10.1.2.3', list));
  assert.ok(ipAllowed('2001:db8::1', list));
  assert.ok(ipAllowed('192.168.1.5', list));
  assert.ok(!ipAllowed('192.168.1.6', list));
  assert.ok(!ipAllowed('11.0.0.1', list));
  assert.ok(!ipAllowed('garbage', list));
  assert.throws(() => buildAllowList(['10.0.0.0/33']));
  assert.throws(() => buildAllowList(['not-an-ip/8']));
});

test('usage tracker reads OpenAI and Anthropic usage, buffered and streamed across chunk splits', () => {
  const openai = new UsageTracker();
  openai.feedJSON(JSON.stringify({ usage: { prompt_tokens: 12, completion_tokens: 7 } }));
  assert.deepEqual([openai.input, openai.output], [12, 7]);

  const anthropic = new UsageTracker();
  const stream = [
    'event: message_start\ndata: {"type":"message_start","message":{"usage":{"input_tokens":25,"output_tokens":1}}}\n\n',
    'event: message_delta\ndata: {"type":"message_delta","usa',
    'ge":{"output_tokens":42}}\n\ndata: [DONE]',
  ];
  for (const c of stream) anthropic.feedSSE(c);
  anthropic.endSSE();
  assert.deepEqual([anthropic.input, anthropic.output], [25, 42]);
});

test('joinURL keeps base path prefixes and does not double /v1', () => {
  assert.equal(joinURL('https://api.openai.com', '/v1/chat/completions').href, 'https://api.openai.com/v1/chat/completions');
  assert.equal(joinURL('https://openrouter.ai/api/', '/v1/chat/completions').href, 'https://openrouter.ai/api/v1/chat/completions');
  assert.equal(joinURL('https://api.groq.com/openai/v1', '/v1/chat/completions').href, 'https://api.groq.com/openai/v1/chat/completions');
});

test('rate limiter enforces RPM with group ratio, and TPM from recorded tokens', () => {
  const rl = new RateLimiterService();
  // rpm 2 × ratio 1.5 = 3 per minute
  for (let i = 0; i < 3; i++) assert.equal(rl.acquire('k', 2, 0, 1.5).retryAfter, 0);
  const limited = rl.acquire('k', 2, 0, 1.5);
  assert.equal(limited.tier, 'rpm');
  assert.ok(limited.retryAfter >= 1 && limited.retryAfter <= 60);

  assert.equal(rl.acquire('t', 0, 100).retryAfter, 0);
  rl.recordTokens('t', 150);
  assert.equal(rl.acquire('t', 0, 100).tier, 'tpm');
  assert.equal(rl.acquire('unlimited', 0, 0).retryAfter, 0);
});

test('channel picker filters by group, model, status, key and client API', () => {
  const registry = new RegistryService();
  registry.upsertChannel(channel('oa', 'openai', ['gpt-4o']), 'k1');
  registry.upsertChannel(channel('an', 'anthropic', ['claude-x'], { 'gpt-4o': 'claude-x' }), 'k2');
  registry.upsertChannel(channel('off', 'openai', [], undefined, 'Disabled'), 'k3');
  registry.upsertChannel(channel('nokey', 'openai', []), undefined);
  registry.upsertGroup(group('team', ['oa', 'an', 'off', 'nokey']));
  const picker = new ChannelPickerService(registry);

  assert.deepEqual(names(picker.candidates('team', 'gpt-4o', 'openai')).sort(), ['an', 'oa']);
  assert.deepEqual(names(picker.candidates('team', 'claude-x', 'openai')), ['an']);
  assert.deepEqual(names(picker.candidates('team', 'gpt-4o', 'anthropic')), ['an']);
  assert.deepEqual(picker.candidates('team', 'unknown-model', 'openai'), []);
  assert.deepEqual(picker.candidates('other-team', 'gpt-4o', 'openai'), []);
  assert.ok(servesModel(registry.getChannel('an')!, 'gpt-4o'));
});

test('status helper keeps transition time and detects real changes only', () => {
  const a = readyStatus(undefined, true, 'Active', 'Active', 1);
  const b = readyStatus(a, true, 'Active', 'Active', 1);
  assert.equal(b.conditions[0].lastTransitionTime, a.conditions[0].lastTransitionTime);
  assert.ok(!statusChanged(a as unknown as Record<string, unknown>, b));
  assert.ok(statusChanged(a as unknown as Record<string, unknown>, readyStatus(a, false, 'Disabled', 'off', 1)));
  assert.ok(statusChanged(a as unknown as Record<string, unknown>, readyStatus(a, true, 'Active', 'Active', 2)));
});

function channel(name: string, type: 'openai' | 'anthropic', models: string[], modelMapping?: Record<string, string>, status: 'Enabled' | 'Disabled' = 'Enabled'): Channel {
  return { metadata: { name }, spec: { type, baseURL: 'http://x', keySecretRef: { name: 's' }, models, modelMapping, status } };
}

function group(name: string, channelRefs: string[]): Group {
  return { metadata: { name }, spec: { channelRefs } };
}

function names(list: Array<{ channel: Channel }>): string[] {
  return list.map((e) => e.channel.metadata!.name!);
}
