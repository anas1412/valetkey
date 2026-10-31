import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'
import { VelocityToast, type VelocityToastVariant } from '@/components/ui/velocity-toast'

interface Toast {
  id: number
  title: string
  description?: string
  variant: VelocityToastVariant
}

type Notify = (title: string, opts?: { description?: string; variant?: VelocityToastVariant }) => void

const ToastContext = createContext<Notify>(() => {})

/** One toast at a time; a new one replaces the current. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toast, setToast] = useState<Toast>()
  const notify = useCallback<Notify>((title, opts) => {
    setToast({ id: Date.now(), title, description: opts?.description, variant: opts?.variant ?? 'success' })
  }, [])

  return (
    <ToastContext.Provider value={notify}>
      {children}
      {toast && (
        <VelocityToast
          key={toast.id}
          open
          title={toast.title}
          description={toast.description}
          variant={toast.variant}
          position="bottom-right"
          onDismiss={() => setToast(undefined)}
        />
      )}
    </ToastContext.Provider>
  )
}

export function useToast(): Notify {
  return useContext(ToastContext)
}
