import { useState } from 'react'
import { Eye, Copy } from 'lucide-react'
import { CodeSnippetDeck } from '@/components/ui/code-snippet-deck'
import { Button } from '@/components/ui/button'
import { api } from '@/lib/api'
import { copyToClipboard } from '@/lib/utils'
import { useToast } from '@/lib/toast'
import { timeAgo } from '@/lib/format'
import type { ProxyKey } from '@/lib/types'

const PLACEHOLDER = 'sk-proxy-…'

/** Expanded row: key status, reveal/copy, and ready-to-paste usage snippets. */
export function KeyDetails({ apiKey, proxyURL }: { apiKey: ProxyKey; proxyURL: string }) {
  const [revealed, setRevealed] = useState<string>()
  const notify = useToast()
  const key = revealed ?? PLACEHOLDER
  const model = apiKey.spec.models?.[0] ?? 'gpt-4o-mini'
  const base = `${proxyURL.replace(/\/+$/, '')}/v1`

  const reveal = async () => {
    try {
      setRevealed((await api.revealKey(apiKey.namespace, apiKey.name)).key)
    } catch (err) {
      notify('Could not read the key', { description: (err as Error).message, variant: 'error' })
    }
  }

  const copy = async () => {
    try {
      const k = revealed ?? (await api.revealKey(apiKey.namespace, apiKey.name)).key
      notify(await copyToClipboard(k) ? 'Key copied' : 'Copy failed', { variant: 'success' })
    } catch (err) {
      notify('Could not read the key', { description: (err as Error).message, variant: 'error' })
    }
  }

  return (
    <div className="space-y-4 py-2">
      <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs text-text-secondary">
        <span>Status: <span className="text-text-primary">{apiKey.message ?? (apiKey.ready ? 'Active' : 'Pending')}</span></span>
        <span>Secret: <code className="text-text-primary">{apiKey.namespace}/{apiKey.secretName ?? `${apiKey.name}-valetkey`}</code></span>
        <span>Requests: <span className="text-text-primary tabular-nums">{apiKey.requests}</span></span>
        <span>Last used: <span className="text-text-primary">{timeAgo(apiKey.lastUsed)}</span></span>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <code className="rounded-md bg-surface-raised border border-border px-2.5 py-1.5 text-xs text-text-primary break-all">
          {revealed ?? `${PLACEHOLDER} (hidden)`}
        </code>
        {!revealed && <Button size="sm" variant="secondary" leftIcon={<Eye className="size-3.5" />} onClick={() => void reveal()} disabled={!apiKey.ready}>Show</Button>}
        <Button size="sm" variant="secondary" leftIcon={<Copy className="size-3.5" />} onClick={() => void copy()} disabled={!apiKey.ready}>Copy</Button>
      </div>

      <CodeSnippetDeck
        showLineNumbers={false}
        snippets={[
          {
            language: 'bash', label: 'cURL',
            code: `curl ${base}/chat/completions \\\n  -H "Authorization: Bearer ${key}" \\\n  -H "Content-Type: application/json" \\\n  -d '{"model":"${model}","messages":[{"role":"user","content":"Hello"}]}'`,
          },
          {
            language: 'python', label: 'Python', filename: 'app.py',
            code: `from openai import OpenAI\n\nclient = OpenAI(base_url="${base}", api_key="${key}")\nreply = client.chat.completions.create(\n    model="${model}",\n    messages=[{"role": "user", "content": "Hello"}],\n)\nprint(reply.choices[0].message.content)`,
          },
          {
            language: 'typescript', label: 'TypeScript', filename: 'app.ts',
            code: `import OpenAI from "openai";\n\nconst client = new OpenAI({ baseURL: "${base}", apiKey: "${key}" });\nconst reply = await client.chat.completions.create({\n  model: "${model}",\n  messages: [{ role: "user", content: "Hello" }],\n});\nconsole.log(reply.choices[0].message.content);`,
          },
          {
            language: 'yaml', label: 'Kubernetes', filename: 'deployment.yaml',
            code: `# In a Deployment in namespace ${apiKey.namespace}:\nenvFrom:\n  - secretRef:\n      name: ${apiKey.secretName ?? `${apiKey.name}-valetkey`}\n# gives the app VALETKEY_KEY and VALETKEY_ENDPOINT`,
          },
        ]}
      />
    </div>
  )
}
