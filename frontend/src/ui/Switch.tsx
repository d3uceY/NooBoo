interface SwitchProps {
  on: boolean
  onChange: (on: boolean) => void
  label: string
  disabled?: boolean
}

/** On/off pill for enabling a mod. */
export function Switch({ on, onChange, label, disabled }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={`relative h-7 w-12 shrink-0 cursor-pointer rounded-full border transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-40 ${
        on
          ? 'border-[#3f7a17] bg-linear-to-b from-[#8ed94f] to-[#57a52a] shadow-[inset_0_1px_0_rgba(255,255,255,0.55)]'
          : 'border-[#a9c1d1] bg-linear-to-b from-[#e3edf4] to-[#cfdde7]'
      }`}
    >
      <span
        className={`absolute top-0.5 h-5 w-5 rounded-full border border-black/10 bg-white shadow-sm transition-all duration-150 ${
          on ? 'left-6' : 'left-0.5'
        }`}
      />
    </button>
  )
}
