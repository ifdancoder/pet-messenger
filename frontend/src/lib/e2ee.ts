import { APIError, api, type Call, type E2EEDevice } from './api'
import type { Message } from '../types'

const DB_NAME = 'pet-messenger-e2ee'
const STORE = 'keys'
const LEGACY_IDENTITY_KEY = 'identity-keypair'
const DEVICE_ID_KEY = 'pet-messenger.e2ee.device-id'
const MESSAGE_PREFIX = 'e2ee:v1:'
const ALGORITHM = 'X25519-HKDF-SHA256+A256GCM'
const encoder = new TextEncoder()
const decoder = new TextDecoder()
const identityInFlight = new Map<string, Promise<StoredIdentity>>()

export type StoredIdentity = { deviceID: string; keyPair: CryptoKeyPair; signingKeyPair: CryptoKeyPair }

type WrappedKey = { n: string; c: string }
type EncryptedMessage = {
  v: 1
  a: typeof ALGORITHM
  d: string
  p: string
  s: string
  n: string
  c: string
  k: Record<string, WrappedKey>
}

function database() {
  return new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1)
    request.onupgradeneeded = () => request.result.createObjectStore(STORE)
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

function identityKey(userID: string) { return `identity:${userID}` }

async function readStoredIdentity(key: string): Promise<StoredIdentity | undefined> {
  const db = await database()
  return await new Promise<StoredIdentity | undefined>((resolve, reject) => {
    const request = db.transaction(STORE, 'readonly').objectStore(STORE).get(key)
    request.onsuccess = () => resolve(request.result as StoredIdentity | undefined)
    request.onerror = () => reject(request.error)
  }).finally(() => db.close())
}

async function saveIdentity(key: string, identity: StoredIdentity) {
  const db = await database()
  await new Promise<void>((resolve, reject) => {
    const request = db.transaction(STORE, 'readwrite').objectStore(STORE).put(identity, key)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error)
  }).finally(() => db.close())
}

async function deleteStoredIdentity(key: string) {
  const db = await database()
  await new Promise<void>((resolve, reject) => {
    const request = db.transaction(STORE, 'readwrite').objectStore(STORE).delete(key)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error)
  }).finally(() => db.close())
}

async function readIdentity(userID: string) {
  const current = await readStoredIdentity(identityKey(userID))
  if (current) return current

  // Builds before per-account identities stored one browser-wide key. Adopt it
  // for the first account that opens the upgraded client, preserving access to
  // messages already encrypted to that device.
  const legacy = await readStoredIdentity(LEGACY_IDENTITY_KEY)
  if (legacy) {
    await saveIdentity(identityKey(userID), legacy)
    await deleteStoredIdentity(LEGACY_IDENTITY_KEY)
    return legacy
  }
  return undefined
}

function base64url(data: ArrayBuffer | Uint8Array) {
  const bytes = data instanceof Uint8Array ? data : new Uint8Array(data)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '')
}

function fromBase64url(value: string): Uint8Array<ArrayBuffer> {
  const normalized = value.replaceAll('-', '+').replaceAll('_', '/')
  const padded = normalized + '='.repeat((4 - normalized.length % 4) % 4)
  const binary = atob(padded)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i)
  return bytes
}

function randomBytes(size: number) { return crypto.getRandomValues(new Uint8Array(size)) }

function equalBytes(left: Uint8Array, right: Uint8Array) {
  if (left.length !== right.length) return false
  let different = 0
  for (let i = 0; i < left.length; i += 1) different |= left[i] ^ right[i]
  return different === 0
}

// The private half is non-extractable and persisted by IndexedDB. It never
// crosses the browser boundary; only this device's raw X25519 public key is
// registered with the API.
async function ensureIdentity(token: string, userID: string, label: string) {
  if (!crypto.subtle || typeof indexedDB === 'undefined') throw new Error('Этот браузер не поддерживает защищённые чаты')
  let identity = await readIdentity(userID)
  if (identity && !identity.signingKeyPair) {
    identity.signingKeyPair = await crypto.subtle.generateKey({ name: 'Ed25519' }, false, ['sign', 'verify']) as CryptoKeyPair
    await saveIdentity(identityKey(userID), identity)
  }
  if (!identity) {
    const deviceID = crypto.randomUUID()
    const keyPair = await crypto.subtle.generateKey({ name: 'X25519' }, false, ['deriveBits']) as CryptoKeyPair
    const signingKeyPair = await crypto.subtle.generateKey({ name: 'Ed25519' }, false, ['sign', 'verify']) as CryptoKeyPair
    identity = { deviceID, keyPair, signingKeyPair }
    await saveIdentity(identityKey(userID), identity)
  }
  localStorage.setItem(`${DEVICE_ID_KEY}.${userID}`, identity.deviceID)
  const publicKey = await crypto.subtle.exportKey('raw', identity.keyPair.publicKey)
  const signingPublicKey = await crypto.subtle.exportKey('raw', identity.signingKeyPair.publicKey)
  await api.registerE2EEDevice({
    id: identity.deviceID, public_key: base64url(publicKey), signing_public_key: base64url(signingPublicKey), label,
  }, token)
  return identity
}

export function ensureE2EEIdentity(token: string, userID: string, label = 'Этот браузер') {
  const pending = identityInFlight.get(userID)
  if (pending) return pending
  const created = ensureIdentity(token, userID, label)
  identityInFlight.set(userID, created)
  void created.finally(() => {
    if (identityInFlight.get(userID) === created) identityInFlight.delete(userID)
  }).catch(() => undefined)
  return created
}

export function e2eeDeviceID(userID: string) { return localStorage.getItem(`${DEVICE_ID_KEY}.${userID}`) }

export async function approveE2EEDevice(target: E2EEDevice, token: string, userID: string) {
  const identity = await ensureE2EEIdentity(token, userID)
  const statement = encoder.encode(`pet-messenger:approve-device:v1:${identity.deviceID}:${target.id}:${target.fingerprint}`)
  const signature = await crypto.subtle.sign({ name: 'Ed25519' }, identity.signingKeyPair.privateKey, statement)
  await api.approveE2EEDevice(target.id, token, identity.deviceID, base64url(signature))
}

function callKeyAAD(callID: string, ownerDeviceID: string) {
  return encoder.encode(`pet-messenger:call-key:v1:${callID}:${ownerDeviceID}`)
}

function callWrappingContext(callID: string, ownerDeviceID: string, recipientDeviceID: string) {
  return `pet-messenger:call-key-wrap:v1:${callID}:${ownerDeviceID}:${recipientDeviceID}`
}

async function createCallKey(call: Call, conversationID: string, token: string, identity: StoredIdentity) {
  const [{ devices }, { members }] = await Promise.all([
    api.conversationE2EEDevices(conversationID, token),
    api.members(conversationID, token),
  ])
  const trusted = trustedDeviceMap(devices)
  if (!trusted.has(identity.deviceID)) throw new Error('Подтвердите это устройство перед звонком')
  const usersWithKeys = new Set(devices.map(device => device.user_id))
  if (members.some(member => !usersWithKeys.has(member.user_id))) {
    throw new Error('Нельзя начать защищённый звонок: не у всех участников настроено E2EE')
  }
  const rawCallKey = randomBytes(32)
  const salt = randomBytes(32)
  const envelopes = await Promise.all([...trusted.values()].map(async device => {
    const wrappingKey = await deriveWrappingKey(
      identity.keyPair.privateKey,
      fromBase64url(device.public_key!),
      salt,
      callWrappingContext(call.id, identity.deviceID, device.id),
    )
    const nonce = randomBytes(12)
    const ciphertext = await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv: nonce, additionalData: callKeyAAD(call.id, identity.deviceID) },
      wrappingKey,
      rawCallKey,
    )
    const envelope = encoder.encode(JSON.stringify({ s: base64url(salt), n: base64url(nonce), c: base64url(ciphertext) }))
    return { recipient_device_id: device.id, ciphertext: base64url(envelope) }
  }))
  await api.putCallKeyEnvelopes(call.id, token, identity.deviceID, envelopes)
  return rawCallKey
}

async function readCallKey(call: Call, conversationID: string, token: string, identity: StoredIdentity, attempts: number) {
  let result: Awaited<ReturnType<typeof api.callKeyEnvelope>> | undefined
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      result = await api.callKeyEnvelope(call.id, token, identity.deviceID)
      break
    } catch (error) {
      if (!(error instanceof APIError) || error.status !== 404 || attempt === attempts - 1) throw error
      await new Promise(resolve => window.setTimeout(resolve, 250))
    }
  }
  if (!result) throw new Error('Ключ защищённого звонка не получен')
  const { devices } = await api.conversationE2EEDevices(conversationID, token)
  const owner = trustedDeviceMap(devices).get(result.owner_device_id)
  if (!owner?.public_key || !equalBytes(fromBase64url(owner.public_key), fromBase64url(result.owner_public_key))) {
    throw new Error('Ключ инициатора звонка не является доверенным')
  }
  const envelope = JSON.parse(decoder.decode(fromBase64url(result.ciphertext))) as { s: string; n: string; c: string }
  const wrappingKey = await deriveWrappingKey(
    identity.keyPair.privateKey,
    fromBase64url(result.owner_public_key),
    fromBase64url(envelope.s),
    callWrappingContext(call.id, result.owner_device_id, identity.deviceID),
  )
  const plaintext = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: fromBase64url(envelope.n), additionalData: callKeyAAD(call.id, result.owner_device_id) },
    wrappingKey,
    fromBase64url(envelope.c),
  )
  return new Uint8Array(plaintext)
}

export async function prepareCallE2EE(call: Call, conversationID: string, token: string, userID: string) {
  if (!('RTCRtpScriptTransform' in window)) throw new Error('Браузер не поддерживает E2EE медиапотоков')
  const identity = await ensureE2EEIdentity(token, userID)
  try {
    return await readCallKey(call, conversationID, token, identity, call.initiator_id === userID ? 1 : 20)
  } catch (error) {
    if (call.initiator_id !== userID || !(error instanceof APIError) || error.status !== 404) throw error
    return createCallKey(call, conversationID, token, identity)
  }
}

async function deriveWrappingKey(privateKey: CryptoKey, publicKey: Uint8Array<ArrayBuffer>, salt: Uint8Array<ArrayBuffer>, info: string) {
  const importedPublicKey = await crypto.subtle.importKey('raw', publicKey, { name: 'X25519' }, false, [])
  const sharedSecret = await crypto.subtle.deriveBits({ name: 'X25519', public: importedPublicKey }, privateKey, 256)
  const material = await crypto.subtle.importKey('raw', sharedSecret, 'HKDF', false, ['deriveKey'])
  return crypto.subtle.deriveKey(
    { name: 'HKDF', hash: 'SHA-256', salt, info: encoder.encode(info) },
    material,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt'],
  )
}

function bodyAAD(conversationID: string, senderDeviceID: string) {
  return encoder.encode(`pet-messenger:message:v1:${conversationID}:${senderDeviceID}`)
}

function wrappingContext(conversationID: string, senderDeviceID: string, recipientDeviceID: string) {
  return `pet-messenger:message-key:v1:${conversationID}:${senderDeviceID}:${recipientDeviceID}`
}

function trustedDeviceMap(devices: E2EEDevice[]) {
  return new Map(devices.filter(device => device.status === 'trusted' && device.public_key).map(device => [device.id, device]))
}

export async function encryptMessage(conversationID: string, plaintext: string, token: string, userID: string) {
  const identity = await ensureE2EEIdentity(token, userID)
  const [{ devices }, { members }] = await Promise.all([
    api.conversationE2EEDevices(conversationID, token),
    api.members(conversationID, token),
  ])
  const trusted = trustedDeviceMap(devices)
  if (!trusted.has(identity.deviceID)) throw new Error('Подтвердите это устройство с уже доверенного устройства')

  const usersWithKeys = new Set(devices.filter(device => device.status === 'trusted').map(device => device.user_id))
  if (members.some(member => !usersWithKeys.has(member.user_id))) {
    throw new Error('Нельзя отправить защищённое сообщение: не у всех участников настроено E2EE')
  }

  const messageKey = await crypto.subtle.generateKey({ name: 'AES-GCM', length: 256 }, true, ['encrypt', 'decrypt'])
  const rawMessageKey = await crypto.subtle.exportKey('raw', messageKey)
  const bodyNonce = randomBytes(12)
  const ciphertext = await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: bodyNonce, additionalData: bodyAAD(conversationID, identity.deviceID) },
    messageKey,
    encoder.encode(plaintext),
  )
  const senderPublicKey = new Uint8Array(await crypto.subtle.exportKey('raw', identity.keyPair.publicKey))
  const salt = randomBytes(32)
  const keys: Record<string, WrappedKey> = {}

  await Promise.all([...trusted.values()].map(async device => {
    const recipientPublicKey = fromBase64url(device.public_key!)
    if (recipientPublicKey.length !== 32) throw new Error('Сервер вернул повреждённый ключ устройства')
    const wrappingKey = await deriveWrappingKey(
      identity.keyPair.privateKey,
      recipientPublicKey,
      salt,
      wrappingContext(conversationID, identity.deviceID, device.id),
    )
    const nonce = randomBytes(12)
    const wrapped = await crypto.subtle.encrypt(
      { name: 'AES-GCM', iv: nonce, additionalData: bodyAAD(conversationID, identity.deviceID) },
      wrappingKey,
      rawMessageKey,
    )
    keys[device.id] = { n: base64url(nonce), c: base64url(wrapped) }
  }))

  const payload: EncryptedMessage = {
    v: 1, a: ALGORITHM, d: identity.deviceID, p: base64url(senderPublicKey),
    s: base64url(salt), n: base64url(bodyNonce), c: base64url(ciphertext), k: keys,
  }
  return MESSAGE_PREFIX + base64url(encoder.encode(JSON.stringify(payload)))
}

function parsePayload(content: string): EncryptedMessage | undefined {
  if (!content.startsWith(MESSAGE_PREFIX)) return undefined
  try {
    const payload = JSON.parse(decoder.decode(fromBase64url(content.slice(MESSAGE_PREFIX.length)))) as EncryptedMessage
    if (payload.v !== 1 || payload.a !== ALGORITHM || !payload.d || !payload.p || !payload.k) throw new Error('invalid envelope')
    return payload
  } catch {
    throw new Error('Повреждённый контейнер защищённого сообщения')
  }
}

async function decryptOne(message: Message, conversationID: string, identity: StoredIdentity, trusted: Map<string, E2EEDevice>) {
  const payload = parsePayload(message.content)
  if (!payload) return message
  const sender = trusted.get(payload.d)
  const senderPublicKey = fromBase64url(payload.p)
  if (!sender?.public_key || !equalBytes(senderPublicKey, fromBase64url(sender.public_key))) {
    throw new Error('Ключ отправителя не является доверенным')
  }
  const wrapped = payload.k[identity.deviceID]
  if (!wrapped) throw new Error('Сообщение было отправлено до подключения этого устройства')
  const wrappingKey = await deriveWrappingKey(
    identity.keyPair.privateKey,
    senderPublicKey,
    fromBase64url(payload.s),
    wrappingContext(conversationID, payload.d, identity.deviceID),
  )
  const rawMessageKey = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: fromBase64url(wrapped.n), additionalData: bodyAAD(conversationID, payload.d) },
    wrappingKey,
    fromBase64url(wrapped.c),
  )
  const messageKey = await crypto.subtle.importKey('raw', rawMessageKey, { name: 'AES-GCM' }, false, ['decrypt'])
  const plaintext = await crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: fromBase64url(payload.n), additionalData: bodyAAD(conversationID, payload.d) },
    messageKey,
    fromBase64url(payload.c),
  )
  return { ...message, content: decoder.decode(plaintext) }
}

export async function decryptMessages(messages: Message[], conversationID: string, token: string, userID: string) {
  if (!messages.some(message => message.content.startsWith(MESSAGE_PREFIX))) return messages
  const identity = await ensureE2EEIdentity(token, userID)
  const { devices } = await api.conversationE2EEDevices(conversationID, token)
  const trusted = trustedDeviceMap(devices)
  return Promise.all(messages.map(async message => {
    try {
      return await decryptOne(message, conversationID, identity, trusted)
    } catch (error) {
      const reason = error instanceof Error ? error.message : 'неизвестная ошибка'
      return { ...message, content: `🔒 Не удалось расшифровать сообщение: ${reason}` }
    }
  }))
}
