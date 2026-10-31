import { Skeleton } from './ui/skeleton'

/** Full-page placeholder while the session is checked. */
export function Loader() {
  return (
    <div className="min-h-dvh bg-background p-6 flex gap-6">
      <Skeleton className="hidden md:block w-60 h-[calc(100dvh-3rem)] rounded-xl" />
      <div className="flex-1 space-y-4">
        <Skeleton className="h-8 w-48" />
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
          {[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-28 rounded-xl" />)}
        </div>
        <Skeleton className="h-72 rounded-xl" />
      </div>
    </div>
  )
}
