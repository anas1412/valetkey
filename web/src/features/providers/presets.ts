import type { ProviderType } from '@/lib/types'

/** Quick-fill choices in the "Add provider" form. "custom" = any OpenAI-compatible endpoint. */
export const PROVIDER_PRESETS: Array<{ id: string; label: string; type: ProviderType; baseURL: string; description: string }> = [
  { id: 'openai', label: 'OpenAI', type: 'openai', baseURL: 'https://api.openai.com', description: 'api.openai.com' },
  { id: 'anthropic', label: 'Anthropic', type: 'anthropic', baseURL: 'https://api.anthropic.com', description: 'api.anthropic.com' },
  { id: 'openrouter', label: 'OpenRouter', type: 'openai', baseURL: 'https://openrouter.ai/api', description: 'OpenAI-compatible' },
  { id: 'groq', label: 'Groq', type: 'openai', baseURL: 'https://api.groq.com/openai', description: 'OpenAI-compatible' },
  { id: 'deepseek', label: 'DeepSeek', type: 'openai', baseURL: 'https://api.deepseek.com', description: 'OpenAI-compatible' },
  { id: 'custom', label: 'Other (OpenAI-compatible)', type: 'openai', baseURL: '', description: 'vLLM, Ollama, Azure, LiteLLM…' },
]

export function presetFor(type: ProviderType, baseURL: string): string {
  return PROVIDER_PRESETS.find((p) => p.type === type && p.baseURL === baseURL)?.id ?? (type === 'anthropic' ? 'anthropic' : 'custom')
}
