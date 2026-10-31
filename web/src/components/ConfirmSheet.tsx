import { useState } from 'react'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetBody } from './ui/sheet'
import { DragToConfirm } from './ui/drag-to-confirm'
import { useToast } from '@/lib/toast'

/** Confirmation for destructive actions: the user drags to confirm instead of a single click. */
export function ConfirmSheet({ open, title, description, onClose, onConfirm, action = 'delete' }: {
  open: boolean
  title: string
  description: string
  onClose: () => void
  onConfirm: () => Promise<void>
  /** delete: "Drag to delete"; rotate: "Drag to rotate" */
  action?: 'delete' | 'rotate'
}) {
  const [busy, setBusy] = useState(false)
  const notify = useToast()

  const confirm = async () => {
    setBusy(true)
    try {
      await onConfirm()
      onClose()
    } catch (err) {
      notify(action === 'delete' ? 'Delete failed' : 'Rotate failed', { description: (err as Error).message, variant: 'error' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right">
        <SheetHeader>
          <SheetTitle>{title}</SheetTitle>
          <SheetDescription>{description}</SheetDescription>
        </SheetHeader>
        <SheetBody>
          <DragToConfirm
            key={String(open)}
            label={action === 'delete' ? 'Drag to delete' : 'Drag to rotate'}
            confirmedLabel={action === 'delete' ? 'Deleting…' : 'Rotating…'}
            actionType={action === 'delete' ? 'delete' : 'confirm'}
            variant={action === 'delete' ? 'danger' : 'warning'}
            disabled={busy}
            onConfirm={() => void confirm()}
          />
        </SheetBody>
      </SheetContent>
    </Sheet>
  )
}
