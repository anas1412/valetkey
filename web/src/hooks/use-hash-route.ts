import { useCallback, useEffect, useState } from 'react'

export const ROUTES = ['overview', 'providers', 'tenants', 'keys', 'pricing', 'activity'] as const
export type Route = (typeof ROUTES)[number] | 'not-found'

function read(): Route {
  const id = location.hash.replace(/^#\/?/, '') || 'overview'
  return (ROUTES as readonly string[]).includes(id) ? (id as Route) : 'not-found'
}

/** Page routing via the URL hash, so browser back/forward and reloads keep the page. */
export function useHashRoute(): [Route, (r: Route) => void] {
  const [route, setRoute] = useState<Route>(read)

  useEffect(() => {
    const onChange = () => setRoute(read())
    addEventListener('hashchange', onChange)
    return () => removeEventListener('hashchange', onChange)
  }, [])

  const navigate = useCallback((r: Route) => {
    location.hash = `/${r}`
  }, [])

  return [route, navigate]
}
