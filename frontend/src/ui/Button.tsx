import type { ButtonHTMLAttributes, ReactNode } from 'react'

type Variant = 'primary' | 'blue' | 'plain' | 'danger'
type Size = 'md' | 'sm'

const variants: Record<Variant, string> = {
  primary:
    'border-[#3f7a17] bg-linear-to-b from-[#8ed94f] to-[#57a52a] text-[#12300f] hover:from-[#9ce45d] hover:to-[#63b333]',
  blue: 'border-[#145c92] bg-linear-to-b from-[#5cbdf3] to-[#1d7fc4] text-white hover:from-[#6ec7f7] hover:to-[#2489d0]',
  plain:
    'border-[#a9c6da] bg-linear-to-b from-white to-[#eaf4fb] text-ink hover:from-white hover:to-[#dceefb]',
  danger:
    'border-[#a13028] bg-linear-to-b from-[#ef7569] to-[#cf4338] text-white hover:from-[#f5836f] hover:to-[#d84c40]',
}

const sizes: Record<Size, string> = {
  md: 'rounded-[10px] px-4 py-2 text-sm',
  sm: 'rounded-md px-2 py-0.5 text-[11px]',
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
  children: ReactNode
}

/**
 * Sims 4 style pill button: gradient fill, light top edge, pressed state.
 * `sm` is the same recipe at row scale, not a different-looking control.
 */
export function Button({ variant = 'plain', size = 'md', className = '', children, ...props }: ButtonProps) {
  return (
    <button
      type="button"
      className={`inline-flex cursor-pointer select-none items-center justify-center gap-2 border font-display font-bold tracking-[0.01em] shadow-[inset_0_1px_0_rgba(255,255,255,0.55),0_2px_6px_rgba(20,50,70,0.18)] transition-[filter,transform] duration-150 hover:brightness-105 active:translate-y-px active:brightness-95 disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:brightness-100 focus-visible:ring-3 focus-visible:ring-sky/50 focus-visible:outline-none ${variants[variant]} ${sizes[size]} ${className}`}
      {...props}
    >
      {children}
    </button>
  )
}
