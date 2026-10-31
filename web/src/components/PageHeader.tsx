import type { ReactNode } from 'react'

export function PageHeader({ title, description, actions }: { title: string; description?: ReactNode; actions?: ReactNode }) {
  return (
    <div className="flex flex-col sm:flex-row sm:items-end sm:justify-between gap-4 mb-6">
      <div>
        <h1 className="text-2xl font-semibold text-text-primary tracking-tight-heading">{title}</h1>
        {description && <p className="mt-1 text-sm text-text-secondary max-w-2xl">{description}</p>}
      </div>
      {actions && <div className="flex gap-2 shrink-0">{actions}</div>}
    </div>
  )
}

/** Bordered surface used for page sections. */
export function Panel({ title, description, children, className = '' }: {
  title?: string
  description?: string
  children: ReactNode
  className?: string
}) {
  return (
    <section className={`rounded-xl border border-border bg-surface p-4 sm:p-5 ${className}`}>
      {title && <h2 className="text-sm font-medium text-text-primary">{title}</h2>}
      {description && <p className="text-xs text-text-muted mt-0.5">{description}</p>}
      <div className={title || description ? 'mt-4' : ''}>{children}</div>
    </section>
  )
}
