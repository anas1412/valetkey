import { joinURL } from '../upstream.js';
import type { Adaptation } from './types.js';

/**
 * OpenAI-compatible passthrough: openai, deepseek, groq, together, openrouter,
 * vLLM, Ollama… The body is forwarded as-is apart from the model name.
 */
export function adaptOpenAI(
  baseURL: string,
  upstreamKey: string,
  body: Record<string, unknown>,
  headers: Record<string, string>,
): Adaptation {
  return {
    request: {
      url: joinURL(baseURL, '/v1/chat/completions'),
      headers: { ...headers, 'content-type': 'application/json', authorization: `Bearer ${upstreamKey}` },
      body: Buffer.from(JSON.stringify(body)),
    },
  };
}
