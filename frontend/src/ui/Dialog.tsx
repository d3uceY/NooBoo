import type { ReactNode } from 'react'
import { useEffect } from 'react'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  footer?: ReactNode
}

/** Centred modal with the game's round close button top-right. */
export function Dialog({ open, onClose, title, children, footer }: DialogProps) {
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[#10394f]/45 backdrop-blur-[2px]"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="w-[min(460px,92vw)] overflow-hidden rounded-2xl border border-[#bcdcef] bg-panel shadow-[0_28px_60px_-20px_rgba(10,40,60,0.7)]"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title}
      >
        <div className="relative border-b border-[#145c92]/40 bg-linear-to-b from-[#5cbdf3] to-[#1d7fc4] px-4 py-3">
          <h2 className="text-center font-display text-base font-bold tracking-wide text-white drop-shadow-[0_1px_1px_rgba(10,50,80,0.55)]">
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="absolute top-1/2 right-2 flex h-6.5 w-6.5 -translate-y-1/2 cursor-pointer items-center justify-center rounded-full border border-[#145c92] bg-white text-xs font-bold text-[#1d7fc4] shadow-sm transition-colors hover:bg-panel-dim"
          >
            ✕
          </button>
        </div>
        <div className="px-4 py-4 text-sm">{children}</div>
        {footer && <div className="flex justify-end gap-2 px-4 pb-4">{footer}</div>}
      </div>
    </div>
  )
}
