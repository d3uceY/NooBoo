interface ProgressBarProps {
  value: number
  label?: string
}

/** Glossy fill bar, used while files are copied. */
export function ProgressBar({ value, label }: ProgressBarProps) {
  const clamped = Math.min(100, Math.max(0, Math.round(value)))
  return (
    <div>
      <div
        className="h-6 overflow-hidden rounded-full border border-[#145c92]/45 bg-[#dceaf4] shadow-[inset_0_2px_4px_rgba(20,50,70,0.22)]"
        role="progressbar"
        aria-valuenow={clamped}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div
          className="h-full rounded-full bg-linear-to-b from-[#a6e06f] to-[#57a52a] shadow-[inset_0_1px_0_rgba(255,255,255,0.7)] transition-[width] duration-200 ease-out"
          style={{ width: `${clamped}%` }}
        />
      </div>
      {label && <p className="mt-2 text-center text-xs text-ink-soft">{label}</p>}
    </div>
  )
}
