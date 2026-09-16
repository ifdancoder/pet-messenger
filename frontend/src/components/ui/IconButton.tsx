import type { ButtonHTMLAttributes, ReactNode } from 'react'

type Props = ButtonHTMLAttributes<HTMLButtonElement> & { label: string; children: ReactNode }

export function IconButton({ label, className = '', children, ...props }: Props) {
  // data-tooltip, not title: a native browser tooltip can't be styled at
  // all, and it's the one hover affordance nearly every icon button in the
  // app relies on — see the [data-tooltip] rule in App.css for the themed
  // popup this actually renders.
  return <button className={`ui-icon-button ${className}`} type="button" aria-label={label} data-tooltip={label} {...props}>{children}</button>
}
