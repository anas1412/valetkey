import { useState, type FormEvent } from 'react'
import { KeyRound } from 'lucide-react'
import { Form, FormItem, FormLabel, FormControl, FormMessage, FormDescription, Input } from '@/components/ui/form'
import { Button } from '@/components/ui/button'
import { Logo } from '@/components/Logo'
import { api } from '@/lib/api'

/** Admin-key sign-in. EasyUI's `login` card is email/password only, so this composes its form parts. */
export function SignIn({ disabled, onSignedIn }: { disabled: boolean; onSignedIn: () => void }) {
  const [key, setKey] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!key) return setError('Enter the admin key')
    setBusy(true)
    setError(undefined)
    try {
      await api.login(key)
      onSignedIn()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="min-h-dvh bg-background flex items-center justify-center px-4">
      <div className="w-full max-w-sm rounded-2xl border border-border bg-surface p-6 sm:p-8 shadow-elevated">
        <div className="flex flex-col items-center text-center mb-6">
          <Logo compact />
          <h1 className="mt-4 text-xl font-semibold text-text-primary tracking-snug-heading">Sign in to LLMP</h1>
          <p className="mt-1.5 text-sm text-text-secondary">Manage providers, tenants and API keys.</p>
        </div>

        {disabled ? (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-text-secondary space-y-2">
            <p className="font-medium text-text-primary">The dashboard is turned off</p>
            <p>Set an admin key, then restart LLMP:</p>
            <pre className="text-xs bg-surface-raised rounded-md p-2 overflow-x-auto">
{`kubectl create secret generic llmproxy-admin \\
  -n llmproxy-system --from-literal=adminKey=<long random string>
kubectl rollout restart deploy/llmproxy -n llmproxy-system`}
            </pre>
          </div>
        ) : (
          <Form onSubmit={submit} className="space-y-4">
            <FormItem>
              <FormLabel required>Admin key</FormLabel>
              <FormControl>
                <Input
                  type="password"
                  autoFocus
                  autoComplete="current-password"
                  showPasswordToggle
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  leftIcon={<KeyRound className="size-3.5" />}
                  error={!!error}
                />
              </FormControl>
              <FormMessage error={error} />
              <FormDescription>The value of the <code>llmproxy-admin</code> Secret.</FormDescription>
            </FormItem>
            <Button type="submit" variant="primary" fullWidth isLoading={busy} loadingText="Signing in…">
              Sign in
            </Button>
          </Form>
        )}
      </div>
    </div>
  )
}
