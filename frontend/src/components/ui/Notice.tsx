import type { ReactNode } from 'react'
export function Notice({ children }: { children: ReactNode }) { return <p className="ui-notice" role="alert">{children}</p> }
