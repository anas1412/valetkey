import { useEffect, useState } from 'react'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetBody } from './ui/sheet'
import { CodeSnippetDeck } from './ui/code-snippet-deck'
import { api, type YamlResource } from '@/lib/api'

export type YamlTarget = { resource: YamlResource; path: string[]; title: string }

// Where each file goes in the repository layout of the GitOps guide.
const purpose: Record<YamlResource, (name: string) => string> = {
  tenants: (name) => `The tenant's whole file: its namespace, tenant and keys. Save it as tenants/${name}.yaml in your Git repository.`,
  providers: (name) => `Save it as providers/${name}.yaml in your Git repository, with the provider's API key, encrypted, at the top.`,
  proxykeys: () => "One key, to paste into its tenant's file in your Git repository.",
}

/** Shows an object as the CRD you would commit to Git or apply with kubectl. */
export function YamlSheet({ target, onClose }: { target?: YamlTarget; onClose: () => void }) {
  const [yaml, setYaml] = useState<string>()
  const [error, setError] = useState<string>()

  useEffect(() => {
    if (!target) return
    setYaml(undefined)
    setError(undefined)
    api.yaml(target.resource, ...target.path).then(setYaml, (err: Error) => setError(err.message))
  }, [target])

  const name = target?.path[target.path.length - 1]
  return (
    <Sheet open={!!target} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right">
        <SheetHeader>
          <SheetTitle>{target?.title}</SheetTitle>
          <SheetDescription>
            {target && name ? purpose[target.resource](name) : ''} Key values are never shown: they stay in Kubernetes Secrets.
          </SheetDescription>
        </SheetHeader>
        <SheetBody>
          {error ? <p className="text-sm text-destructive">{error}</p>
            : yaml === undefined ? <p className="text-sm text-text-muted">Loading…</p>
            : <CodeSnippetDeck showLineNumbers={false} snippets={[{ language: 'yaml', label: 'YAML', filename: `${name}.yaml`, code: yaml }]} />}
        </SheetBody>
      </SheetContent>
    </Sheet>
  )
}
