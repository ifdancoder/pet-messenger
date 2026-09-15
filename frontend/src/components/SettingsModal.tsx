import { useState } from 'react'
import type { Session } from '../types'
import { Button, IconButton, SelectField, TextField } from './ui'
import { api } from '../lib/api'
import { loadDevicePrefs, saveDevicePrefs } from '../lib/devicePrefs'

type Tab = 'profile' | 'devices' | 'audio'

type Props = {
  session: Session
  onSessionUpdate: (session: Session) => void
  onClose: () => void
  isCallActive: boolean
  switchDevice: (kind: 'audioinput' | 'videoinput', deviceId: string) => Promise<void>
  setNoiseSuppression: (enabled: boolean) => Promise<void>
}

export function SettingsModal({ session, onSessionUpdate, onClose, isCallActive, switchDevice, setNoiseSuppression }: Props) {
  const [tab, setTab] = useState<Tab>('profile')
  const [displayName, setDisplayName] = useState(session.user.display_name)
  const [username, setUsername] = useState(session.user.username)
  const [error, setError] = useState('')
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([])
  const [prefs, setPrefs] = useState(loadDevicePrefs())

  const loadDevices = async () => {
    await navigator.mediaDevices.getUserMedia({ audio: true, video: true }).then(s => s.getTracks().forEach(t => t.stop())).catch(() => {})
    setDevices(await navigator.mediaDevices.enumerateDevices())
  }

  const saveProfile = async () => {
    setError('')
    try {
      const updated = await api.updateProfile({ display_name: displayName, username }, session.access_token)
      onSessionUpdate({ ...session, user: { ...session.user, display_name: updated.display_name, username: updated.username } })
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось сохранить профиль')
    }
  }

  const onAvatarChange = async (file: File | null) => {
    if (!file) return
    setError('')
    try {
      await api.uploadAvatar(file, session.access_token)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить аватар')
    }
  }

  const onDeviceChange = async (kind: 'audioinput' | 'videoinput', deviceId: string) => {
    const next = { ...prefs, [kind === 'audioinput' ? 'audioInputId' : 'videoInputId']: deviceId }
    setPrefs(next)
    saveDevicePrefs(next)
    if (isCallActive) await switchDevice(kind, deviceId)
  }

  const onNoiseSuppressionChange = async (enabled: boolean) => {
    setPrefs({ ...prefs, noiseSuppression: enabled })
    await setNoiseSuppression(enabled)
  }

  return <div className="settings-modal-backdrop" onClick={onClose}>
    <section className="settings-modal" role="dialog" aria-modal="true" aria-label="Настройки" onClick={event => event.stopPropagation()}>
      <header><h2>Настройки</h2><IconButton label="Закрыть" onClick={onClose}>✕</IconButton></header>
      <nav className="settings-modal__tabs">
        <button type="button" className={tab === 'profile' ? 'is-active' : ''} onClick={() => setTab('profile')}>Профиль</button>
        <button type="button" className={tab === 'devices' ? 'is-active' : ''} onClick={() => { setTab('devices'); void loadDevices() }}>Устройства</button>
        <button type="button" className={tab === 'audio' ? 'is-active' : ''} onClick={() => setTab('audio')}>Звук</button>
      </nav>
      {error ? <p className="settings-modal__error">{error}</p> : null}
      {tab === 'profile' ? <div className="settings-modal__panel">
        <label>Отображаемое имя<TextField value={displayName} onChange={e => setDisplayName(e.target.value)} /></label>
        <label>Имя пользователя<TextField value={username} onChange={e => setUsername(e.target.value)} /></label>
        <label>Аватар<input type="file" accept="image/png,image/jpeg,image/webp" onChange={e => void onAvatarChange(e.target.files?.[0] ?? null)} /></label>
        <Button onClick={() => void saveProfile()}>Сохранить</Button>
      </div> : null}
      {tab === 'devices' ? <div className="settings-modal__panel">
        <label>Микрофон
          <SelectField value={prefs.audioInputId ?? ''} onChange={e => void onDeviceChange('audioinput', e.target.value)}>
            {devices.filter(d => d.kind === 'audioinput').map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || d.deviceId}</option>)}
          </SelectField>
        </label>
        <label>Камера
          <SelectField value={prefs.videoInputId ?? ''} onChange={e => void onDeviceChange('videoinput', e.target.value)}>
            {devices.filter(d => d.kind === 'videoinput').map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || d.deviceId}</option>)}
          </SelectField>
        </label>
      </div> : null}
      {tab === 'audio' ? <div className="settings-modal__panel">
        <label className="settings-modal__toggle">
          <input type="checkbox" checked={prefs.noiseSuppression} onChange={e => void onNoiseSuppressionChange(e.target.checked)} />
          Шумоподавление (RNNoise)
        </label>
      </div> : null}
    </section>
  </div>
}
