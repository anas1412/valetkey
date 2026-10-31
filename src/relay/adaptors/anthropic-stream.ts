import { finishReason } from './anthropic-translate.js';
import type { StreamTranslator } from './types.js';

type Json = Record<string, any>;

/**
 * Translates an Anthropic Messages SSE stream into OpenAI chat.completion.chunk SSE.
 * Anthropic sends one JSON object per `data:` line, each carrying its own `type`.
 */
export class AnthropicToOpenAIStream implements StreamTranslator {
  private partial = '';
  private id = '';
  private model = '';
  private readonly created = Math.floor(Date.now() / 1000);
  /** Anthropic content block index → OpenAI tool_calls index */
  private toolIndex = new Map<number, number>();
  private done = false;

  push(chunk: string): string {
    const lines = (this.partial + chunk).split('\n');
    this.partial = lines.pop() ?? '';
    return lines.map((l) => this.line(l)).join('');
  }

  end(): string {
    const out = this.line(this.partial);
    this.partial = '';
    return this.done ? out : out + 'data: [DONE]\n\n';
  }

  private line(line: string): string {
    if (!line.startsWith('data:')) return '';
    let ev: Json;
    try {
      ev = JSON.parse(line.slice(5).trim());
    } catch {
      return '';
    }

    switch (ev.type) {
      case 'message_start':
        this.id = ev.message?.id ?? '';
        this.model = ev.message?.model ?? '';
        return this.chunk({ role: 'assistant', content: '' });

      case 'content_block_start':
        if (ev.content_block?.type === 'tool_use') {
          const idx = this.toolIndex.size;
          this.toolIndex.set(ev.index, idx);
          return this.chunk({
            tool_calls: [{
              index: idx, id: ev.content_block.id, type: 'function',
              function: { name: ev.content_block.name, arguments: '' },
            }],
          });
        }
        return '';

      case 'content_block_delta':
        if (ev.delta?.type === 'text_delta') return this.chunk({ content: ev.delta.text });
        if (ev.delta?.type === 'input_json_delta') {
          const idx = this.toolIndex.get(ev.index) ?? 0;
          return this.chunk({ tool_calls: [{ index: idx, function: { arguments: ev.delta.partial_json } }] });
        }
        return '';

      case 'message_delta':
        return this.chunk({}, finishReason(ev.delta?.stop_reason));

      case 'message_stop':
        this.done = true;
        return 'data: [DONE]\n\n';

      case 'error':
        return sse({ error: { message: ev.error?.message ?? 'upstream error', type: ev.error?.type ?? 'upstream_error' } });

      default:
        return '';
    }
  }

  private chunk(delta: Json, finish: string | null = null): string {
    return sse({
      id: this.id,
      object: 'chat.completion.chunk',
      created: this.created,
      model: this.model,
      choices: [{ index: 0, delta, finish_reason: finish }],
    });
  }
}

function sse(obj: unknown): string {
  return `data: ${JSON.stringify(obj)}\n\n`;
}
