import { api } from './api'

const DB_NAME = 'pet-messenger-e2ee'
const STORE = 'keys'
const DEVICE_ID_KEY = 'pet-messenger.e2ee.device-id'
const KEYPAIR_KEY = 'identity-keypair'

type StoredIdentity = { deviceID: string; keyPair: CryptoKeyPair }

function database() {
  return new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1)
    request.onupgradeneeded = () => request.result.createObjectStore(STORE)
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

async function readIdentity(): Promise<StoredIdentity | undefined> {
  const db = await database()
  return await new Promise<StoredIdentity | undefined>((resolve, reject) => {
    const request = db.transaction(STORE, 'readonly').objectStore(STORE).get(KEYPAIR_KEY)
    request.onsuccess = () => resolve(request.result as StoredIdentity | undefined)
    request.onerror = () => reject(request.error)
  }).finally(() => db.close())
}

async function saveIdentity(identity: StoredIdentity) {
  const db = await database()
  await new Promise<void>((resolve, reject) => {
    const request = db.transaction(STORE, 'readwrite').objectStore(STORE).put(identity, KEYPAIR_KEY)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error)
  }).finally(() => db.close())
}

function base64url(data: ArrayBuffer) {
  const bytes = new Uint8Array(data)
  let binary = ''
  bytes.forEach(byte => { binary += String.fromCharCode(byte) })
  return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replaceAll('=', '')
}

// The private half is a non-extractable CryptoKey persisted by IndexedDB. It
// never crosses the browser boundary; only this device's raw X25519 public key
// is registered with the API.
export async function ensureE2EEIdentity(token: string, label = 'Этот браузер') {
  if (!crypto.subtle) throw new Error('Этот браузер не поддерживает защищённые чаты')
  let identity = await readIdentity()
  if (!identity) {
    const deviceID = crypto.randomUUID()
    const keyPair = await crypto.subtle.generateKey({ name: 'X25519' }, false, ['deriveBits']) as CryptoKeyPair
    identity = { deviceID, keyPair }
    await saveIdentity(identity)
    localStorage.setItem(DEVICE_ID_KEY, deviceID)
  }
  const publicKey = await crypto.subtle.exportKey('raw', identity.keyPair.publicKey)
  await api.registerE2EEDevice({ id: identity.deviceID, public_key: base64url(publicKey), label }, token)
  return identity
}

export function e2eeDeviceID() { return localStorage.getItem(DEVICE_ID_KEY) }
