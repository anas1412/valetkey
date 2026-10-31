import { RelayError } from '../errors.js';

/**
 * OpenAI Chat Completions ⇄ Anthropic Messages translation (non-streaming parts).
 * Covers text, images, tools/tool results, system prompts and sampling params.
 */

type Json = Record<string, unknown>;
type Block = Json & { type: string };

interface OpenAIMessage {
  role: string;
  content?: string | Json[] | null;
  tool_calls?: Array<{ id: string; function: { name: string; arguments?: string } }>;
  tool_call_id?: string;
}

export function openAIRequestToAnthropic(body: Json): Json {
  const messages = body.messages;
  if (!Array.isArray(messages)) throw new RelayError(400, 'invalid_request_error', '"messages" must be an array');

  const system: string[] = [];
  const out: Array<{ role: 'user' | 'assistant'; content: Block[] }> = [];
  const push = (role: 'user' | 'assistant', blocks: Block[]): void => {
    if (!blocks.length) return;
    const prev = out[out.length - 1];
    // Anthropic wants alternating turns; merge consecutive same-role messages.
    if (prev?.role === role) prev.content.push(...blocks);
    else out.push({ role, content: blocks });
  };

  for (const m of messages as OpenAIMessage[]) {
    switch (m.role) {
      case 'system':
      case 'developer':
        system.push(textOf(m.content));
        break;
      case 'user':
        push('user', contentBlocks(m.content));
        break;
      case 'assistant': {
        const blocks = contentBlocks(m.content);
        for (const tc of m.tool_calls ?? []) {
          blocks.push({ type: 'tool_use', id: tc.id, name: tc.function.name, input: parseArgs(tc.function.arguments) });
        }
        push('assistant', blocks);
        break;
      }
      case 'tool':
        push('user', [{ type: 'tool_result', tool_use_id: m.tool_call_id, content: textOf(m.content) }]);
        break;
    }
  }

  const req: Json = {
    model: body.model,
    messages: out,
    max_tokens: body.max_completion_tokens ?? body.max_tokens ?? 4096,
  };
  if (system.length) req.system = system.join('\n\n');
  if (body.temperature !== undefined) req.temperature = body.temperature;
  if (body.top_p !== undefined) req.top_p = body.top_p;
  if (body.stop !== undefined && body.stop !== null) req.stop_sequences = Array.isArray(body.stop) ? body.stop : [body.stop];
  if (body.stream !== undefined) req.stream = body.stream;
  if (typeof body.user === 'string') req.metadata = { user_id: body.user };

  if (Array.isArray(body.tools) && body.tools.length) {
    req.tools = (body.tools as Array<{ function: { name: string; description?: string; parameters?: Json } }>)
      .map((t) => ({
        name: t.function.name,
        description: t.function.description,
        input_schema: t.function.parameters ?? { type: 'object', properties: {} },
      }));
    const choice = toolChoice(body.tool_choice);
    if (choice) req.tool_choice = choice;
  }
  return req;
}

export function anthropicResponseToOpenAI(body: unknown, status: number): unknown {
  const b = body as Json;
  if (status >= 400 || b?.type === 'error') {
    const err = (b?.error ?? {}) as { type?: string; message?: string };
    return { error: { message: err.message ?? 'upstream error', type: err.type ?? 'upstream_error', code: err.type ?? null } };
  }

  let text = '';
  const toolCalls: Json[] = [];
  for (const block of (b.content as Block[] | undefined) ?? []) {
    if (block.type === 'text') text += block.text as string;
    if (block.type === 'tool_use') {
      toolCalls.push({
        id: block.id,
        type: 'function',
        function: { name: block.name, arguments: JSON.stringify(block.input ?? {}) },
      });
    }
  }
  const usage = (b.usage ?? {}) as { input_tokens?: number; output_tokens?: number };
  const input = usage.input_tokens ?? 0;
  const output = usage.output_tokens ?? 0;

  return {
    id: b.id,
    object: 'chat.completion',
    created: Math.floor(Date.now() / 1000),
    model: b.model,
    choices: [{
      index: 0,
      message: {
        role: 'assistant',
        content: text || null,
        ...(toolCalls.length ? { tool_calls: toolCalls } : {}),
      },
      finish_reason: finishReason(b.stop_reason as string | undefined),
    }],
    usage: { prompt_tokens: input, completion_tokens: output, total_tokens: input + output },
  };
}

export function finishReason(stop: string | null | undefined): string | null {
  switch (stop) {
    case 'end_turn':
    case 'stop_sequence':
      return 'stop';
    case 'max_tokens':
      return 'length';
    case 'tool_use':
      return 'tool_calls';
    case 'refusal':
      return 'content_filter';
    default:
      return stop ?? null;
  }
}

function contentBlocks(content: OpenAIMessage['content']): Block[] {
  if (content === null || content === undefined) return [];
  if (typeof content === 'string') return content ? [{ type: 'text', text: content }] : [];
  const blocks: Block[] = [];
  for (const part of content) {
    if (part.type === 'text') blocks.push({ type: 'text', text: part.text });
    if (part.type === 'image_url') blocks.push(imageBlock((part.image_url as { url: string }).url));
  }
  return blocks;
}

function imageBlock(url: string): Block {
  const m = /^data:([^;]+);base64,(.*)$/s.exec(url);
  if (m) return { type: 'image', source: { type: 'base64', media_type: m[1], data: m[2] } };
  return { type: 'image', source: { type: 'url', url } };
}

function textOf(content: OpenAIMessage['content']): string {
  if (typeof content === 'string') return content;
  if (!content) return '';
  return content.filter((p) => p.type === 'text').map((p) => p.text as string).join('\n');
}

function parseArgs(args: string | undefined): unknown {
  if (!args) return {};
  try {
    return JSON.parse(args);
  } catch {
    throw new RelayError(400, 'invalid_request_error', 'tool_calls[].function.arguments is not valid JSON');
  }
}

function toolChoice(choice: unknown): Json | undefined {
  if (choice === 'auto') return { type: 'auto' };
  if (choice === 'required') return { type: 'any' };
  if (choice === 'none') return { type: 'none' };
  const name = (choice as { function?: { name?: string } } | undefined)?.function?.name;
  return name ? { type: 'tool', name } : undefined;
}
