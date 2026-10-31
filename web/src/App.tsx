import { useHashRoute } from './hooks/use-hash-route'
import { AppShell } from './features/shell/AppShell'
import { OverviewPage } from './features/overview/OverviewPage'
import { ProvidersPage } from './features/providers/ProvidersPage'
import { TenantsPage } from './features/tenants/TenantsPage'
import { KeysPage } from './features/keys/KeysPage'
import { ActivityPage } from './features/activity/ActivityPage'
import { PricingPage } from './features/pricing/PricingPage'
import { NotFound } from './components/ui/not-found'

/** No sign-in: the dashboard is only reachable through `kubectl port-forward`. */
export function App() {
  const [route, navigate] = useHashRoute()

  return (
    <AppShell route={route} onNavigate={navigate}>
      {route === 'overview' && <OverviewPage onNavigate={navigate} />}
      {route === 'providers' && <ProvidersPage />}
      {route === 'tenants' && <TenantsPage />}
      {route === 'keys' && <KeysPage />}
      {route === 'pricing' && <PricingPage />}
      {route === 'activity' && <ActivityPage />}
      {route === 'not-found' && (
        <NotFound
          title="This page doesn't exist"
          description="The dashboard has Overview, Activity, Providers, Tenants, API keys and Pricing."
          actionLabel="Go to Overview"
          onAction={() => navigate('overview')}
          showBack={false}
        />
      )}
    </AppShell>
  )
}
