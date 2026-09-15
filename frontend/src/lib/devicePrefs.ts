const STORAGE_KEY = 'pet-messenger.devices.v1'

export type DevicePrefs = { audioInputId?: string; videoInputId?: string; noiseSuppression: boolean }

const defaults: DevicePrefs = { noiseSuppression: false }

// Per-machine preference, not account state — deliberately never touches
// the backend, unlike the profile fields in Part A.
export function loadDevicePrefs(): DevicePrefs {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...defaults }
    return { ...defaults, ...JSON.parse(raw) }
  } catch {
    return { ...defaults }
  }
}

export function saveDevicePrefs(prefs: DevicePrefs) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(prefs))
  } catch {
    // Private browsing / storage disabled: the setting just doesn't
    // persist across reloads, which is a reasonable degrade.
  }
}
