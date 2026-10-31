import { test } from 'node:test';
import assert from 'node:assert/strict';
import { openAIRequestToAnthropic, anthropicResponseToOpenAI } from '../src/relay/adaptors/anthropic-translate.js';
import { AnthropicToOpenAIStream } from '../src/relay/adaptors/anthropic-stream.js';

test('OpenAI request → Anthropic: system, images, tools, tool results, params', () => {
  const out = openAIRequestToAnthropic({
    model: 'claude-x',
    max_tokens: 100,
    temperature: 0.2,
    stop: 'END',
    stream: true,
    messages: [
      { role: 'system', content: 'be brief' },
      { role: 'user', content: [{ type: 'text', text: 'what is this?' }, { type: 'image_url', image_url: { url: 'data:image/png;base64,AAAA' } }] },
      { role: 'assistant', content: null, tool_calls: [{ id: 't1', type: 'function', function: { name: 'look', arguments: '{"q":1}' } }] },
      { role: 'tool', tool_call_id: 't1', content: 'a cat' },
      { role: 'user', content: 'thanks' },
    ],
    tools: [{ type: 'function', function: { name: 'look', description: 'd', parameters: { type: 'object' } } }],
    tool_choice: 'required',
  }) as any;

  assert.equal(out.system, 'be brief');
  assert.equal(out.max_tokens, 100);
  assert.deepEqual(out.stop_sequences, ['END']);
  assert.equal(out.stream, true);
  assert.deepEqual(out.tool_choice, { type: 'any' });
  assert.equal(out.tools[0].input_schema.type, 'object');
  assert.equal(out.messages.length, 3); // tool result + following user message merged into one user turn
  assert.deepEqual(out.messages[0].content[1], { type: 'image', source: { type: 'base64', media_type: 'image/png', data: 'AAAA' } });
  assert.deepEqual(out.messages[1].content[0], { type: 'tool_use', id: 't1', name: 'look', input: { q: 1 } });
  assert.equal(out.messages[2].content[0].type, 'tool_result');
  assert.equal(out.messages[2].content[1].text, 'thanks');
});

test('Anthropic response → OpenAI chat.completion, including tool calls and errors', () => {
  const ok = anthropicResponseToOpenAI({
    id: 'msg_1', model: 'claude-x', stop_reason: 'tool_use',
    content: [{ type: 'text', text: 'hi' }, { type: 'tool_use', id: 't1', name: 'look', input: { q: 1 } }],
    usage: { input_tokens: 5, output_tokens: 3 },
  }, 200) as any;
  assert.equal(ok.object, 'chat.completion');
  assert.equal(ok.choices[0].message.content, 'hi');
  assert.equal(ok.choices[0].finish_reason, 'tool_calls');
  assert.equal(ok.choices[0].message.tool_calls[0].function.arguments, '{"q":1}');
  assert.deepEqual(ok.usage, { prompt_tokens: 5, completion_tokens: 3, total_tokens: 8 });

  const err = anthropicResponseToOpenAI({ type: 'error', error: { type: 'overloaded_error', message: 'busy' } }, 529) as any;
  assert.deepEqual(err.error, { message: 'busy', type: 'overloaded_error', code: 'overloaded_error' });
});

test('Anthropic SSE → OpenAI chunks, split at awkward boundaries', () => {
  const events = [
    { type: 'message_start', message: { id: 'msg_1', model: 'claude-x', usage: { input_tokens: 5 } } },
    { type: 'content_block_start', index: 0, content_block: { type: 'text', text: '' } },
    { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'Hel' } },
    { type: 'content_block_delta', index: 0, delta: { type: 'text_delta', text: 'lo' } },
    { type: 'content_block_start', index: 1, content_block: { type: 'tool_use', id: 't1', name: 'look' } },
    { type: 'content_block_delta', index: 1, delta: { type: 'input_json_delta', partial_json: '{"q":' } },
    { type: 'message_delta', delta: { stop_reason: 'end_turn' }, usage: { output_tokens: 2 } },
    { type: 'message_stop' },
  ];
  const raw = events.map((e) => `event: ${e.type}\ndata: ${JSON.stringify(e)}\n\n`).join('');
  const t = new AnthropicToOpenAIStream();
  let out = '';
  for (let i = 0; i < raw.length; i += 7) out += t.push(raw.slice(i, i + 7));
  out += t.end();

  const chunks = out.split('\n\n').filter(Boolean).map((l) => l.replace(/^data: /, ''));
  assert.equal(chunks.at(-1), '[DONE]');
  assert.equal(chunks.filter((c) => c === '[DONE]').length, 1);
  const parsed = chunks.slice(0, -1).map((c) => JSON.parse(c));
  assert.equal(parsed[0].choices[0].delta.role, 'assistant');
  assert.equal(parsed.map((p) => p.choices[0].delta.content ?? '').join(''), 'Hello');
  const tool = parsed.find((p) => p.choices[0].delta.tool_calls?.[0]?.id);
  assert.equal(tool.choices[0].delta.tool_calls[0].function.name, 'look');
  assert.equal(parsed.at(-1).choices[0].finish_reason, 'stop');
  assert.ok(parsed.every((p) => p.id === 'msg_1' && p.object === 'chat.completion.chunk'));
});
