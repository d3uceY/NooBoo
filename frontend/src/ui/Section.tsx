import type { ReactNode } from 'react'
import { useState } from 'react'
import { Art } from './icons'
import type { ArtName } from './icons'

interface SectionProps {
  title: string
  count: number
  art: ArtName
  defaultOpen?: boolean
  children: ReactNode
}

/** Green collapsible header, the same shape the game uses for Traits. */
export function Section({ title, count, art, defaultOpen = true, children }: SectionProps) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div className="mb-3">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="sticky top-0 z-10 flex w-full cursor-pointer items-center gap-2 rounded-lg border border-[#3f7a17] bg-linear-to-b from-[#8ed94f] to-[#59a72a] px-3 py-1.5 text-left shadow-[inset_0_1px_0_rgba(255,255,255,0.5),0_2px_4px_rgba(20,50,70,0.18)] transition-[filter] hover:brightness-105"
      >
        <Art name={art} size={18} />
        <span className="font-display text-xs font-bold tracking-wider text-white uppercase drop-shadow-[0_1px_1px_rgba(30,70,10,0.5)]">
          {title}
        </span>
        <span className="rounded-full border border-white/40 bg-white/25 px-2 text-[10px] font-bold text-white">
          {count}
        </span>
        <span
          className={`ml-auto text-sm text-white transition-transform duration-200 ${open ? 'rotate-90' : ''}`}
          aria-hidden="true"
        >
          ▶
        </span>
      </button>

      {open && (
        <div className="mt-1 divide-y divide-[#eef5fa] overflow-hidden rounded-xl border border-[#bcdcef] bg-white p-1.5 shadow-[inset_0_2px_4px_rgba(20,50,70,0.06)]">
          {children}
        </div>
      )}
    </div>
  )
}
