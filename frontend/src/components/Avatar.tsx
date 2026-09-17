import { useEffect, useState } from 'react'
import { apiURL } from '../lib/api'

// A single shape every avatar site in the app renders from — a person's own
// fields, never anything picked at random. avatarUrl is the API's own
// relative path (e.g. "/users/alice/avatar"); resolveSrc below turns that
// into a fetchable URL.
export type AvatarSubject = { avatarUrl?: string; displayName?: string; username?: string }

function resolveSrc(avatarUrl?: string) {
  if (!avatarUrl) return undefined
  return /^https?:\/\//.test(avatarUrl) ? avatarUrl : apiURL + avatarUrl
}

// Renders the uploaded photo when there is one and it actually loads;
// otherwise (no avatarUrl, or the fetch fails) falls back to a plain initial
// letter derived from the same subject — so this can never end up showing an
// unrelated placeholder image, only what the person's own data says.
// className supplies the sizing/shape (e.g. "identity__avatar"), shared
// between the <img> and the fallback <span> so either one drops into the
// same slot.
//
// The avatar route sits behind the same bearer-token auth as the rest of the
// API (see AuthMiddleware), and a plain <img src> can't carry an
// Authorization header — so this fetches the image itself and hands the
// <img> an object URL instead of the raw API path.
export function Avatar({ subject, token, className }: { subject: AvatarSubject; token: string; className?: string }) {
  const [objectUrl, setObjectUrl] = useState<string>()
  const [failed, setFailed] = useState(false)
  const src = resolveSrc(subject.avatarUrl)
  useEffect(() => {
    setFailed(false)
    setObjectUrl(undefined)
    if (!src) return
    let current = true
    let created: string | undefined
    void fetch(src, { headers: { Authorization: `Bearer ${token}` } })
      .then(response => { if (!response.ok) throw new Error('avatar fetch failed'); return response.blob() })
      .then(blob => { if (!current) return; created = URL.createObjectURL(blob); setObjectUrl(created) })
      .catch(() => { if (current) setFailed(true) })
    return () => { current = false; if (created) URL.revokeObjectURL(created) }
  }, [src, token])
  const label = (subject.displayName || subject.username || '').trim()
  const initial = (label.slice(0, 1) || '?').toUpperCase()
  const classes = className ? `avatar ${className}` : 'avatar'
  if (objectUrl && !failed) return <img className={classes} src={objectUrl} alt="" onError={() => setFailed(true)} />
  return <span className={classes} aria-hidden="true">{initial}</span>
}
