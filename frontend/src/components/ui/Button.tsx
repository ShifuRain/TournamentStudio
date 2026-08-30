import type { ButtonHTMLAttributes } from 'react'

type Variant = 'primary' | 'outline' | 'ghost'
type Size = 'md' | 'sm'

const base = 'rounded-md transition-colors disabled:cursor-not-allowed disabled:opacity-40'

const variants: Record<Variant, string> = {
  primary: 'bg-yellow text-navy font-display font-extrabold uppercase tracking-wide hover:bg-yellow/90',
  outline: 'border border-hairline text-foam hover:border-foam-dim',
  ghost: 'font-mono text-red-tint hover:text-red',
}

const sizes: Record<Size, string> = {
  md: 'px-4 py-2 text-sm',
  sm: 'px-3 py-1 text-xs',
}

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  size?: Size
}

export function Button({ variant = 'primary', size = 'md', className = '', ...props }: ButtonProps) {
  return <button className={`${base} ${variants[variant]} ${sizes[size]} ${className}`} {...props} />
}
