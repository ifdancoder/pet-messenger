import { useState } from 'react'
import type { FormEvent } from 'react'
import { Button, SelectField, TextField } from './ui'

type Props = { onCreate: (kind: 'direct' | 'group', value: string) => Promise<boolean> }

export function NewConversationForm({ onCreate }: Props) {
  const [kind, setKind] = useState<'direct' | 'group'>('direct')
  const [isSubmitting, setIsSubmitting] = useState(false)
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = event.currentTarget
    const value = String(new FormData(form).get('value') ?? '').trim()
    if (!value) return
    setIsSubmitting(true)
    const created = await onCreate(kind, value)
    setIsSubmitting(false)
    if (created) form.reset()
  }
  return <form className="new-conversation-form" onSubmit={submit}>
    <SelectField value={kind} onChange={event => setKind(event.target.value as 'direct' | 'group')} aria-label="Тип диалога"><option value="direct">Личный диалог</option><option value="group">Группа</option></SelectField>
    <TextField name="value" placeholder={kind === 'direct' ? 'Логин пользователя' : 'Название группы'} required />
    <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Создаю…' : 'Создать'}</Button>
  </form>
}
