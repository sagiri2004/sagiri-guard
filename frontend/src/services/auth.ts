const TOKEN_KEY = 'admin_token'
const DEVICE_KEY = 'admin_device'
const DEFAULT_HOST = '127.0.0.1'
const DEFAULT_PORT = 9200

type SendCommandFn = (
  host: string,
  port: number,
  req: { device_id: string; token?: string; action: string; data: string },
) => Promise<{ ok: boolean; status_code: number; status_msg: string; error?: string }>

declare global {
  interface Window {
    sendCommand?: SendCommandFn
  }
}

export function loadToken(): string | null {
  if (typeof localStorage === 'undefined') return null
  return localStorage.getItem(TOKEN_KEY)
}

export function loadDeviceId(): string | null {
  if (typeof localStorage === 'undefined') return null
  return localStorage.getItem(DEVICE_KEY)
}

export async function loginAdmin(username: string, password: string, deviceId: string): Promise<string> {
  if (!username || !password) {
    throw new Error('Missing credentials')
  }
  if (!window.sendCommand) {
    throw new Error('sendCommand bridge is not available')
  }
  const payload = {
    username,
    password,
    device_id: deviceId,
  }
  const res = await window.sendCommand(DEFAULT_HOST, DEFAULT_PORT, {
    device_id: deviceId,
    action: 'login',
    data: JSON.stringify(payload),
  })
  if (res.error) {
    throw new Error(res.error)
  }
  if (!res.ok) {
    throw new Error(res.status_msg || `Login failed (${res.status_code})`)
  }
  let token = ''
  let returnedDevice = deviceId
  try {
    const obj = JSON.parse(res.status_msg || '{}')
    token = obj.token || ''
    returnedDevice = obj.device_id || deviceId
  } catch {
    // ignore parse error
  }
  if (!token) {
    throw new Error('Invalid login response')
  }
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem(TOKEN_KEY, token)
    if (returnedDevice) localStorage.setItem(DEVICE_KEY, returnedDevice)
  }
  return token
}

export function logout() {
  if (typeof localStorage === 'undefined') return
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(DEVICE_KEY)
}

