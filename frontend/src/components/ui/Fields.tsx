import type { InputHTMLAttributes, SelectHTMLAttributes } from 'react'

export function TextField({ className = '', ...props }: InputHTMLAttributes<HTMLInputElement>) { return <input className={`ui-field ${className}`} {...props} /> }
export function SelectField({ className = '', ...props }: SelectHTMLAttributes<HTMLSelectElement>) { return <select className={`ui-field ui-select ${className}`} {...props} /> }
