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

const DEFAULT_HOST = '127.0.0.1'
const DEFAULT_PORT = 9200

type Session = {
  token: string
  deviceId: string
}

let session: Session | null = null

async function ensureAdminSession(): Promise<Session> {
  if (session && session.token && session.deviceId) return session
  ensureBridge()
  // synth device id for admin UI
  const deviceId = 'admin-ui'
  const payload = { username: 'admin', password: 'admin123', device_id: deviceId }
  const res = await window.sendCommand!(DEFAULT_HOST, DEFAULT_PORT, {
    device_id: deviceId,
    action: 'login',
    data: JSON.stringify(payload),
  })
  if (res.error) throw new Error(res.error)
  if (!res.ok) throw new Error(res.status_msg || `status ${res.status_code}`)
  let token = ''
  try {
    const obj = JSON.parse(res.status_msg || '{}')
    token = obj.token || ''
  } catch {
    /* ignore */
  }
  if (!token) throw new Error('invalid login response (token empty)')
  session = { token, deviceId }
  return session
}

export type DeviceSummary = {
  uuid: string
  name: string
  online: boolean
}

function ensureBridge() {
  if (!window.sendCommand) {
    throw new Error('sendCommand bridge is not available')
  }
}

export async function fetchDevices(): Promise<DeviceSummary[]> {
  const sess = await ensureAdminSession()
  const res = await window.sendCommand!(DEFAULT_HOST, DEFAULT_PORT, {
    device_id: sess.deviceId,
    token: sess.token,
    action: 'admin_list_devices',
    data: '{}',
  })
  if (res.error) throw new Error(res.error)
  if (!res.ok) throw new Error(res.status_msg || `status ${res.status_code}`)
  const body = (res.status_msg || '').trim()
  if (!body) return []
  try {
    return JSON.parse(body)
  } catch {
    // fallback: no devices if not JSON
    return []
  }
}

export async function fetchOnline(): Promise<string[]> {
  const sess = await ensureAdminSession()
  const res = await window.sendCommand!(DEFAULT_HOST, DEFAULT_PORT, {
    device_id: sess.deviceId,
    token: sess.token,
    action: 'admin_list_online',
    data: '{}',
  })
  if (res.error) throw new Error(res.error)
  if (!res.ok) throw new Error(res.status_msg || `status ${res.status_code}`)
  const body = (res.status_msg || '').trim()
  if (!body) return []
  try {
    return JSON.parse(body)
  } catch {
    return []
  }
}

export async function sendAdminCommand(params: {
  deviceId: string
  action: string
  payload: string
  token?: string
}): Promise<{ status: string; sent: boolean; id?: number }> {
  ensureBridge()
  const { deviceId, action, payload, token } = params
  if (!deviceId || !action) throw new Error('missing deviceId or action')
  const sess = await ensureAdminSession()
  const res = await window.sendCommand!(DEFAULT_HOST, DEFAULT_PORT, {
    device_id: sess.deviceId,
    token: token || sess.token,
    action: 'admin_send_command',
    data: JSON.stringify({
      device_id: deviceId,
      command: action,
      payload: payload ? JSON.parse(payload) : {},
    }),
  })
  if (res.error) throw new Error(res.error)
  if (!res.ok) throw new Error(res.status_msg || `status ${res.status_code}`)
  try {
    return JSON.parse(res.status_msg || '{}')
  } catch {
    throw new Error('invalid response')
  }
}

