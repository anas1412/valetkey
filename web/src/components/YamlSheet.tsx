import { useEffect, useState } from 'react'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetBody } from './ui/sheet'
import { CodeSnippetDeck } from './ui/code-snippet-deck'
import { api, type YamlResource } from '@/lib/api'

export type YamlTarget = { resource: YamlResource; path: string[]; title: string }

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
            The live object as a CRD, ready to commit to Git or apply with kubectl. Key values are not in it: they stay in Kubernetes Secrets.
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
