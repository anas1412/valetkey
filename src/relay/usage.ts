/**
 * Token usage extraction from upstream responses, for both OpenAI-compatible
 * and Anthropic payloads, buffered or streamed (SSE).
 */
export class UsageTracker {
  input = 0;
  output = 0;
  private partial = '';

  /** Feed a complete (non-streaming) JSON response body. */
  feedJSON(text: string): void {
    try {
      this.read(JSON.parse(text));
    } catch {}
  }

  /** Feed a raw SSE chunk; lines may be split across chunks. */
  feedSSE(chunk: string): void {
    const lines = (this.partial + chunk).split('\n');
    this.partial = lines.pop() ?? '';
    for (const line of lines) this.readLine(line);
  }

  /** Flush the trailing partial line at end of stream. */
  endSSE(): void {
    this.readLine(this.partial);
    this.partial = '';
  }

  private readLine(line: string): void {
    if (!line.startsWith('data:')) return;
    const data = line.slice(5).trim();
    if (!data || data === '[DONE]') return;
    try {
      this.read(JSON.parse(data));
    } catch {}
  }

  private read(obj: unknown): void {
    if (!obj || typeof obj !== 'object') return;
    const o = obj as { usage?: Usage; message?: { usage?: Usage } };
    // Anthropic message_start carries usage under .message; message_delta and
    // OpenAI final chunks carry it at the top level. Counts are cumulative.
    for (const u of [o.usage, o.message?.usage]) {
      if (!u) continue;
      const input = u.prompt_tokens ?? u.input_tokens;
      const output = u.completion_tokens ?? u.output_tokens;
      if (typeof input === 'number' && input > this.input) this.input = input;
      if (typeof output === 'number' && output > this.output) this.output = output;
    }
  }
}

interface Usage {
  prompt_tokens?: number;
  completion_tokens?: number;
  input_tokens?: number;
  output_tokens?: number;
}
