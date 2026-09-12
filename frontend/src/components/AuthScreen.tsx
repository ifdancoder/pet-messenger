import { useState } from 'react'
import type { FormEvent } from 'react'
import { api } from '../lib/api'
import { Button, Notice, TextField } from './ui'
import type { Session } from '../types'
import { Brand } from './Brand'

type Props = { onAuthenticated: (session: Session) => void }

export function AuthScreen({ onAuthenticated }: Props) {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [error, setError] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const isRegistering = mode === 'register'

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const username = String(form.get('username') ?? '').trim()
    const password = String(form.get('password') ?? '')
    const displayName = String(form.get('display_name') ?? '').trim()
    setError(''); setIsSubmitting(true)
    try { onAuthenticated(isRegistering ? await api.register(displayName, username, password) : await api.login(username, password)) }
    catch (error) { setError(error instanceof Error ? error.message : 'Не удалось продолжить') }
    finally { setIsSubmitting(false) }
  }

  return <main className="auth-page"><section className="auth-card">
    <Brand />
    <div><h1>{isRegistering ? 'Создайте свой уголок' : 'С возвращением'}</h1><p>{isRegistering ? 'Один аккаунт для личных и групповых разговоров.' : 'Войдите, чтобы продолжить разговор.'}</p></div>
    <form className="auth-form" onSubmit={submit}>
      {isRegistering ? <TextField name="display_name" placeholder="Как вас зовут" autoComplete="name" required /> : null}
      <TextField name="username" placeholder="Логин" autoComplete="username" required />
      <TextField name="password" type="password" placeholder="Пароль" autoComplete={isRegistering ? 'new-password' : 'current-password'} minLength={8} required />
      <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Подождите…' : isRegistering ? 'Создать аккаунт' : 'Войти'}</Button>
    </form>
    {error ? <Notice>{error}</Notice> : null}
    <button className="auth-switch" type="button" onClick={() => { setMode(isRegistering ? 'login' : 'register'); setError('') }}>{isRegistering ? 'Уже есть аккаунт? Войти' : 'Нет аккаунта? Зарегистрироваться'}</button>
  </section></main>
}
