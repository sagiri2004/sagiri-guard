import { useMemo, useState } from 'react'
import './CommandPage.css'

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

type Result = {
  ok: boolean
  statusCode?: number
  statusMsg?: string
  error?: string
}

const templates: Record<string, string> = {
  ping: '{}',
  login: JSON.stringify({ username: 'admin', password: 'admin123', device_id: 'DEVICE_ID_HERE' }, null, 2),
  filetree_sync: '[]',
  agent_log: JSON.stringify({ lines: 'hello from webui' }, null, 2),
}

const CommandPage = () => {
  const [host, setHost] = useState('127.0.0.1')
  const [port, setPort] = useState(9200)
  const [deviceId, setDeviceId] = useState('')
  const [token, setToken] = useState('')
  const [action, setAction] = useState('ping')
  const [data, setData] = useState(templates.ping)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<Result | null>(null)

  const canSend = useMemo(() => !!host && port > 0 && !!action && !!deviceId, [host, port, action, deviceId])

  const applyTemplate = (act: string) => {
    setAction(act)
    setData(templates[act] ?? '{}')
  }

  const send = async () => {
    setResult(null)
    if (!window.sendCommand) {
      setResult({ ok: false, error: 'sendCommand bridge is not available' })
      return
    }
    if (!canSend) {
      setResult({ ok: false, error: 'Missing host/port/action/device_id' })
      return
    }
    setLoading(true)
    try {
      const res = await window.sendCommand(host, port, {
        device_id: deviceId.trim(),
        token: token.trim() || undefined,
        action: action.trim(),
        data: data || '{}',
      })
      setResult({
        ok: res.ok,
        statusCode: res.status_code,
        statusMsg: res.status_msg,
        error: res.error,
      })
    } catch (err: any) {
      setResult({ ok: false, error: err?.message || String(err) })
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="cmd-page">
      <div className="cmd-card">
        <h2>Send Command to Protocol Server</h2>
        <div className="row">
          <label>Host</label>
          <input value={host} onChange={(e) => setHost(e.target.value)} />
          <label>Port</label>
          <input type="number" value={port} onChange={(e) => setPort(Number(e.target.value))} />
        </div>
        <div className="row">
          <label>Device ID</label>
          <input value={deviceId} onChange={(e) => setDeviceId(e.target.value)} placeholder="required" />
        </div>
        <div className="row">
          <label>Token (JWT)</label>
          <input value={token} onChange={(e) => setToken(e.target.value)} placeholder="optional" />
        </div>
        <div className="row">
          <label>Action</label>
          <select value={action} onChange={(e) => setAction(e.target.value)}>
            <option value="ping">ping</option>
            <option value="login">login</option>
            <option value="filetree_sync">filetree_sync</option>
            <option value="agent_log">agent_log</option>
            <option value="backup_init_upload">backup_init_upload</option>
            <option value="backup_init_download">backup_init_download</option>
            <option value="backup_download_start">backup_download_start</option>
            <option value="custom">custom</option>
          </select>
          <button type="button" onClick={() => applyTemplate(action)}>
            Template
          </button>
        </div>
        <div className="row column">
          <label>Data (JSON)</label>
          <textarea value={data} onChange={(e) => setData(e.target.value)} />
        </div>
        <div className="row actions">
          <button onClick={send} disabled={loading || !canSend}>
            {loading ? 'Sending...' : 'Send'}
          </button>
          <span className="hint">Need device_id and (usually) token; login first to get token.</span>
        </div>
      </div>

      <div className="cmd-card">
        <h3>Result</h3>
        {!result && <div className="muted">No result yet</div>}
        {result && (
          <pre className={result.ok ? 'ok' : 'err'}>
{`ok: ${result.ok}
status_code: ${result.statusCode ?? ''}
status_msg: ${result.statusMsg ?? ''}
error: ${result.error ?? ''}`}
          </pre>
        )}
      </div>
    </div>
  )
}

export default CommandPage

