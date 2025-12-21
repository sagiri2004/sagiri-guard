import { useState } from 'react'
import './LoginPage.css'

type Props = {
  onLogin: (username: string, password: string, deviceId: string) => Promise<void>
}

const LoginPage = ({ onLogin }: Props) => {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [deviceId, setDeviceId] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setLoading(true)
    try {
      await onLogin(username, password, deviceId)
    } catch (err: any) {
      setError(err?.message || 'Login failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-container">
      <form className="login-card" onSubmit={handleSubmit}>
        <h2>Admin Login</h2>
        <label>
          Username
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="admin"
            required
          />
        </label>
        <label>
          Device ID
          <input
            value={deviceId}
            onChange={(e) => setDeviceId(e.target.value)}
            placeholder="device id (required)"
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="••••••"
            required
          />
        </label>
        {error && <div className="login-error">{error}</div>}
        <button type="submit" disabled={loading}>
          {loading ? 'Signing in...' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}

export default LoginPage

