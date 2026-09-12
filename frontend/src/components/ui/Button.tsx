import type { ButtonHTMLAttributes, ReactNode } from 'react'

type Props = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'ghost' | 'danger'; children: ReactNode }

export function Button({ variant = 'primary', className = '', type = 'button', children, ...props }: Props) {
  return <button className={`ui-button ui-button--${variant} ${className}`} type={type} {...props}>{children}</button>
}
