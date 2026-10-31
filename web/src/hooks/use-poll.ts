import { useCallback, useEffect, useRef, useState } from 'react'

/**
 * Loads data and refreshes it every `intervalMs` while the tab is visible.
 * `reload()` refetches immediately (after a create/update/delete).
 */
export function usePoll<T>(load: () => Promise<T>, intervalMs = 5000) {
  const [data, setData] = useState<T>()
  const [error, setError] = useState<string>()
  const loadRef = useRef(load)
  loadRef.current = load

  const reload = useCallback(async () => {
    try {
      setData(await loadRef.current())
      setError(undefined)
    } catch (err) {
      setError((err as Error).message)
    }
  }, [])

  useEffect(() => {
    void reload()
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') void reload()
    }, intervalMs)
    return () => clearInterval(id)
  }, [reload, intervalMs])

  return { data, error, reload, loading: data === undefined && error === undefined }
}
