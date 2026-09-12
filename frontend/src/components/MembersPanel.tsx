import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api } from '../lib/api'
import type { Conversation, MemberProfile, Session } from '../types'
import { Button, IconButton, Notice, Spinner, TextField } from './ui'
import './MembersPanel.css'

type Props = { conversation: Conversation; session: Session; onClose: () => void }
export function MembersPanel({ conversation, session, onClose }: Props) {
  const [members, setMembers] = useState<MemberProfile[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isInviting, setIsInviting] = useState(false)
  const [removingUserID, setRemovingUserID] = useState('')
  const [error, setError] = useState('')
  const load = useCallback(async () => {
    setIsLoading(true)
    try { setMembers((await api.members(conversation.id, session.access_token)).members) }
    catch (error) { setError(error instanceof Error ? error.message : 'Не удалось загрузить участников') }
    finally { setIsLoading(false) }
  }, [conversation.id, session.access_token])
  useEffect(() => { void Promise.resolve().then(load) }, [load])
  async function invite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const form = event.currentTarget
    const username = String(new FormData(form).get('username') ?? '').trim(); if (!username) return
    setIsInviting(true); setError('')
    try { const user = await api.userByUsername(username, session.access_token); await api.addMember(conversation.id, user.id, session.access_token); form.reset(); await load() }
    catch (error) { setError(error instanceof Error ? error.message : 'Не удалось пригласить участника') }
    finally { setIsInviting(false) }
  }
  async function removeMember(member: MemberProfile) {
    setRemovingUserID(member.user_id); setError('')
    try { await api.removeMember(conversation.id, member.user_id, session.access_token); await load() }
    catch (error) { setError(error instanceof Error ? error.message : 'Не удалось удалить участника') }
    finally { setRemovingUserID('') }
  }
  const currentMember = members.find(member => member.user_id === session.user.id)
  const canManage = Boolean(currentMember?.admin_tier)
  return <aside className="members-panel"><header><div><span>Группа</span><h2>Участники</h2></div><IconButton label="Закрыть" onClick={onClose}>×</IconButton></header>{canManage ? <form className="members-invite" onSubmit={invite}><TextField name="username" placeholder="Логин пользователя" required /><Button type="submit" disabled={isInviting}>{isInviting ? '…' : 'Пригласить'}</Button></form> : null}{error ? <Notice>{error}</Notice> : null}<div className="members-list">{isLoading ? <div className="members-loading"><Spinner /></div> : members.map(member => <article className="member" key={member.user_id}><span className="member__avatar">{(member.display_name || member.username).slice(0, 1).toUpperCase()}</span><div className="member__body"><b>{member.display_name || member.username}</b><small>@{member.username} · {member.role}</small></div>{canManage && member.user_id !== session.user.id ? <Button className="member__remove" variant="danger" disabled={removingUserID === member.user_id} onClick={() => void removeMember(member)}>{removingUserID === member.user_id ? '…' : 'Удалить'}</Button> : null}</article>)}</div></aside>
}
