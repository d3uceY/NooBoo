interface FieldProps {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

/** Labelled text input, matching the game's sunken field style. */
export function Field({ label, value, onChange, placeholder }: FieldProps) {
  return (
    <label className="block">
      <span className="mb-1 block text-[11px] font-semibold text-ink-soft">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-lg border border-[#a9c6da] bg-white px-3 py-2 font-sans text-sm text-ink shadow-[inset_0_2px_3px_rgba(20,50,70,0.12)] transition-colors placeholder:text-[#9fb6c6] focus:border-[#1d7fc4] focus:ring-2 focus:ring-sky/30 focus:outline-none"
      />
    </label>
  )
}
