import { joinURL } from '../upstream.js';
import { openAIRequestToAnthropic, anthropicResponseToOpenAI } from './anthropic-translate.js';
import { AnthropicToOpenAIStream } from './anthropic-stream.js';
import type { Adaptation } from './types.js';

const DEFAULT_VERSION = '2023-06-01';

/**
 * Anthropic Messages API channel.
 *   - Anthropic clients (/v1/messages): body passed through.
 *   - OpenAI clients (/v1/chat/completions): request, response and stream are translated.
 */
export function adaptAnthropic(
  baseURL: string,
  upstreamKey: string,
  body: Record<string, unknown>,
  headers: Record<string, string>,
  fromOpenAI: boolean,
): Adaptation {
  const request = {
    url: joinURL(baseURL, '/v1/messages'),
    headers: {
      ...headers,
      'content-type': 'application/json',
      'x-api-key': upstreamKey,
      'anthropic-version': headers['anthropic-version'] ?? DEFAULT_VERSION,
    },
    body: Buffer.from(JSON.stringify(fromOpenAI ? openAIRequestToAnthropic(body) : body)),
  };
  if (!fromOpenAI) return { request };
  return {
    request,
    translateJSON: anthropicResponseToOpenAI,
    createStreamTranslator: () => new AnthropicToOpenAIStream(),
  };
}
