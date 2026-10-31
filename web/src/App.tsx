import { useEffect, useState } from 'react'
import { api, setUnauthorizedHandler, ApiError } from './lib/api'
import { useHashRoute } from './hooks/use-hash-route'
import { SignIn } from './features/auth/SignIn'
import { AppShell } from './features/shell/AppShell'
import { OverviewPage } from './features/overview/OverviewPage'
import { ProvidersPage } from './features/providers/ProvidersPage'
import { TenantsPage } from './features/tenants/TenantsPage'
import { KeysPage } from './features/keys/KeysPage'
import { ActivityPage } from './features/activity/ActivityPage'
import { NotFound } from './components/ui/not-found'
import { Loader } from './components/Loader'

type Auth = 'checking' | 'signed-out' | 'signed-in' | 'disabled'

export function App() {
  const [auth, setAuth] = useState<Auth>('checking')
  const [route, navigate] = useHashRoute()

  useEffect(() => {
    setUnauthorizedHandler(() => setAuth('signed-out'))
    api.session()
      .then(() => setAuth('signed-in'))
      .catch((err) => setAuth(err instanceof ApiError && err.status === 503 ? 'disabled' : 'signed-out'))
  }, [])

  if (auth === 'checking') return <Loader />
  if (auth !== 'signed-in') return <SignIn disabled={auth === 'disabled'} onSignedIn={() => setAuth('signed-in')} />

  const signOut = async () => {
    await api.logout().catch(() => {})
    setAuth('signed-out')
  }

  return (
    <AppShell route={route} onNavigate={navigate} onSignOut={signOut}>
      {route === 'overview' && <OverviewPage onNavigate={navigate} />}
      {route === 'providers' && <ProvidersPage />}
      {route === 'tenants' && <TenantsPage />}
      {route === 'keys' && <KeysPage />}
      {route === 'activity' && <ActivityPage />}
      {route === 'not-found' && (
        <NotFound
          title="This page doesn't exist"
          description="The dashboard has Overview, Providers, Tenants, Keys and Activity."
          actionLabel="Go to Overview"
          onAction={() => navigate('overview')}
          showBack={false}
        />
      )}
    </AppShell>
  )
}
