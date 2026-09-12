import type { ButtonHTMLAttributes, ReactNode } from 'react'

type Props = ButtonHTMLAttributes<HTMLButtonElement> & { label: string; children: ReactNode }

export function IconButton({ label, className = '', children, ...props }: Props) {
  return <button className={`ui-icon-button ${className}`} type="button" aria-label={label} title={label} {...props}>{children}</button>
}
