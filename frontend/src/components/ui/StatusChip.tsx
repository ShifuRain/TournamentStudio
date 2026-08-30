import type { ReactNode } from 'react'

type Tone = 'neutral' | 'warn' | 'good' | 'accent'

const toneClasses: Record<Tone, string> = {
  neutral: 'border-hairline text-slate',
  warn: 'border-red-tint/40 bg-red/10 text-red-tint',
  good: 'border-teal-tint/40 bg-teal/10 text-teal-tint',
  accent: 'border-yellow/40 bg-yellow/10 text-yellow',
}

export function StatusChip({ tone = 'neutral', children }: { tone?: Tone; children: ReactNode }) {
  return (
    <span
      className={`inline-flex items-center rounded border px-2 py-0.5 font-mono text-xs font-semibold uppercase tracking-wide ${toneClasses[tone]}`}
    >
      {children}
    </span>
  )
}
