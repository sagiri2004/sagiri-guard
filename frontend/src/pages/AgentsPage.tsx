import { useEffect, useState } from 'react'
import type { DeviceSummary } from '../services/admin'
import { fetchDevices, fetchOnline, sendAdminCommand } from '../services/admin'
import './AgentsPage.css'

type CommandResult = {
  ok: boolean
  message: string
}

const actionTemplates: Record<string, string> = {
  ping: '{}',
  agent_log: JSON.stringify({ lines: 'hello from webui' }, null, 2),
  filetree_sync: '[]',
}

const AgentsPage = () => {
  const [devices, setDevices] = useState<DeviceSummary[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [selected, setSelected] = useState<DeviceSummary | null>(null)
  const [action, setAction] = useState('ping')
  const [payload, setPayload] = useState(actionTemplates.ping)
  const [token, setToken] = useState('')
  const [result, setResult] = useState<CommandResult | null>(null)

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      const [ds, online] = await Promise.all([fetchDevices(), fetchOnline()])
      const onlineSet = new Set(online)
      const merged = ds.map((d) => ({ ...d, online: onlineSet.has(d.uuid) }))
      setDevices(merged)
      if (!selected && ds.length > 0) {
        setSelected(merged[0])
      } else if (selected) {
        const found = merged.find((d) => d.uuid === selected.uuid)
        setSelected(found || null)
      }
    } catch (err: any) {
      setError(err?.message || 'Load failed')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    const id = setInterval(load, 10000) // auto-refresh online status
    return () => clearInterval(id)
  }, [])

  const applyTemplate = (act: string) => {
    setAction(act)
    setPayload(actionTemplates[act] ?? '{}')
  }

  const send = async () => {
    if (!selected) {
      setResult({ ok: false, message: 'Select a device' })
      return
    }
    setResult(null)
    try {
      const res = await sendAdminCommand({
        deviceId: selected.uuid,
        action,
        payload,
        token: token || undefined,
      })
      setResult({ ok: true, message: `status=${res.status} sent=${res.sent} id=${res.id ?? ''}` })
    } catch (err: any) {
      setResult({ ok: false, message: err?.message || 'Send failed' })
    }
  }

  return (
    <div className="agents-layout">
      <div className="sidebar">
        <div className="sidebar-header">
          <h3>Devices</h3>
          <button onClick={load} disabled={loading}>
            {loading ? 'Loading...' : 'Refresh'}
          </button>
        </div>
        {error && <div className="error">{error}</div>}
        <ul className="device-list">
          {devices.map((d) => (
            <li
              key={d.uuid}
              className={selected?.uuid === d.uuid ? 'active' : ''}
              onClick={() => setSelected(d)}
            >
              <div className="dev-name">{d.name || d.uuid}</div>
              <div className={d.online ? 'badge online' : 'badge offline'}>
                {d.online ? 'online' : 'offline'}
              </div>
            </li>
          ))}
          {devices.length === 0 && !loading && <div className="muted">No devices</div>}
        </ul>
      </div>

      <div className="content">
        <h2>Device Detail</h2>
        {!selected && <div className="muted">Select a device</div>}
        {selected && (
          <>
            <div className="detail-grid">
              <div><strong>UUID:</strong> {selected.uuid}</div>
              <div><strong>Name:</strong> {selected.name || '-'}</div>
              <div><strong>Status:</strong> {selected.online ? 'online' : 'offline'}</div>
            </div>

            <div className="cmd-card">
              <h3>Send Command</h3>
              <div className="row">
                <label>Token (optional)</label>
                <input value={token} onChange={(e) => setToken(e.target.value)} />
              </div>
              <div className="row">
                <label>Action</label>
                <select value={action} onChange={(e) => setAction(e.target.value)}>
                  <option value="ping">ping</option>
                  <option value="get_logs">get_logs</option>
                  <option value="backup_auto">backup_auto</option>
                  <option value="restore">restore</option>
                  <option value="block_website">block_website</option>
                  <option value="custom">custom</option>
                </select>
                <button type="button" onClick={() => applyTemplate(action)}>Template</button>
              </div>
              <div className="row column">
                <label>Payload (JSON)</label>
                <textarea value={payload} onChange={(e) => setPayload(e.target.value)} />
              </div>
              <div className="row actions">
                <button onClick={send} disabled={loading}>{loading ? 'Sending...' : 'Send'}</button>
                <span className="hint">Device must be online to send immediately; otherwise queued.</span>
              </div>
              {result && (
                <div className={result.ok ? 'result ok' : 'result err'}>
                  {result.message}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
  )
}

export default AgentsPage

