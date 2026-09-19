import { useState } from 'react'
import type { Session } from '../types'
import { Avatar } from './Avatar'
import { Button, IconButton, SelectField, TextField } from './ui'
import { api, type E2EEDevice } from '../lib/api'
import { loadDevicePrefs, saveDevicePrefs } from '../lib/devicePrefs'
import { approveE2EEDevice, ensureE2EEIdentity } from '../lib/e2ee'

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
  const [mediaDevices, setMediaDevices] = useState<MediaDeviceInfo[]>([])
  const [secureDevices, setSecureDevices] = useState<E2EEDevice[]>([])
  const [prefs, setPrefs] = useState(loadDevicePrefs())

  // Only primes permissions/enumerates once per modal session — every device
  // list refresh briefly flashes the camera/mic indicator (the permission
  // grant it needs), which doing on every tab click would make needlessly
  // noisy for no benefit once the list is already populated.
  const loadDevices = async () => {
    if (mediaDevices.length > 0) return
    await navigator.mediaDevices.getUserMedia({ audio: true, video: true }).then(s => s.getTracks().forEach(t => t.stop())).catch(() => {})
    setMediaDevices(await navigator.mediaDevices.enumerateDevices())
  }

  const loadSecureDevices = async () => {
    try {
      await ensureE2EEIdentity(session.access_token, session.user.id)
      const result = await api.e2eeDevices(session.access_token)
      setSecureDevices(result.devices)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось загрузить защищённые устройства')
    }
  }

  const approveSecureDevice = async (device: E2EEDevice) => {
    setError('')
    try {
      await approveE2EEDevice(device, session.access_token, session.user.id)
      await loadSecureDevices()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Не удалось подтвердить устройство')
    }
  }

  const switchTab = (next: Tab) => { setTab(next); setError('') }

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
      const updated = await api.uploadAvatar(file, session.access_token)
      // The path itself doesn't change on a re-upload, so a plain
      // avatar_url update wouldn't budge either React's own re-render (same
      // src string) or the browser's 5-minute avatar cache. Tagging it with
      // a version makes this and every other place that renders this
      // session's own avatar fetch the new bytes right away.
      onSessionUpdate({ ...session, user: { ...session.user, avatar_url: `${updated.avatar_url}?v=${Date.now()}` } })
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
        <button type="button" className={tab === 'profile' ? 'is-active' : ''} onClick={() => switchTab('profile')}>Профиль</button>
        <button type="button" className={tab === 'devices' ? 'is-active' : ''} onClick={() => { switchTab('devices'); void loadDevices(); void loadSecureDevices() }}>Устройства</button>
        <button type="button" className={tab === 'audio' ? 'is-active' : ''} onClick={() => switchTab('audio')}>Звук</button>
      </nav>
      {error ? <p className="settings-modal__error">{error}</p> : null}
      {tab === 'profile' ? <div className="settings-modal__panel">
        <label>Отображаемое имя<TextField value={displayName} onChange={e => setDisplayName(e.target.value)} /></label>
        <label>Имя пользователя<TextField value={username} onChange={e => setUsername(e.target.value)} /></label>
        <label className="settings-modal__avatar-field">Аватар
          <span className="settings-modal__avatar-row">
            <Avatar subject={{ avatarUrl: session.user.avatar_url, displayName: session.user.display_name, username: session.user.username }} token={session.access_token} className="settings-modal__avatar" />
            <input type="file" accept="image/png,image/jpeg,image/webp" onChange={e => void onAvatarChange(e.target.files?.[0] ?? null)} />
          </span>
        </label>
        <Button onClick={() => void saveProfile()}>Сохранить</Button>
      </div> : null}
      {tab === 'devices' ? <div className="settings-modal__panel">
        <label>Микрофон
          <SelectField value={prefs.audioInputId ?? ''} onChange={e => void onDeviceChange('audioinput', e.target.value)}>
            {mediaDevices.filter(d => d.kind === 'audioinput').map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || d.deviceId}</option>)}
          </SelectField>
        </label>
        <label>Камера
          <SelectField value={prefs.videoInputId ?? ''} onChange={e => void onDeviceChange('videoinput', e.target.value)}>
            {mediaDevices.filter(d => d.kind === 'videoinput').map(d => <option key={d.deviceId} value={d.deviceId}>{d.label || d.deviceId}</option>)}
          </SelectField>
        </label>
        <div className="settings-modal__secure-devices">
          <strong>Защищённые устройства</strong>
          <p>Новое устройство получает ключи чатов только после подтверждения с доверенного устройства.</p>
          {secureDevices.map(device => <div className="settings-modal__secure-device" key={device.id}>
            <span>
              <b>{device.label || 'Устройство'}</b>
              <small>{device.fingerprint.match(/.{1,4}/g)?.slice(0, 8).join(' ')}</small>
            </span>
            {device.status === 'pending'
              ? <Button onClick={() => void approveSecureDevice(device)}>Подтвердить</Button>
              : <span className="settings-modal__device-trusted">Доверенное</span>}
          </div>)}
        </div>
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
