import type { ReactNode } from 'react'

interface PanelProps {
  children: ReactNode
  className?: string
}

/** White Sims 4 panel with a soft lift and a hairline light edge. */
export function Panel({ children, className = '' }: PanelProps) {
  return (
    <div
      className={`overflow-hidden rounded-2xl border border-[#bcdcef] bg-panel shadow-[0_16px_38px_-26px_rgba(23,60,90,0.55),inset_0_1px_0_rgba(255,255,255,0.9)] ${className}`}
    >
      {children}
    </div>
  )
}

interface PanelHeaderProps {
  title: string
  hint?: string
  children?: ReactNode
}

/** Blue gradient header bar, the shape the game uses for its dialogs. */
export function PanelHeader({ title, hint, children }: PanelHeaderProps) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-[#145c92]/40 bg-linear-to-b from-[#5cbdf3] to-[#1d7fc4] px-4 py-2.5">
      <div className="min-w-0">
        <h2 className="truncate font-display text-sm font-bold tracking-wide text-white drop-shadow-[0_1px_1px_rgba(10,50,80,0.55)]">
          {title}
        </h2>
        {hint && <p className="truncate text-[11px] text-white/80">{hint}</p>}
      </div>
      {children}
    </div>
  )
}
