import { useEffect, useState } from 'react'
import { api, ApiError, getUser } from '../lib/api'
import { relTime } from '../lib/fmt'

type User = {
  id: number
  email: string
  full_name: string
  role: string
  is_active: boolean
  created_at: string
}

type Settings = { portal_title: string | null; portal_message: string | null }

type Page<T> = { items: T[]; total: number; page: number; size: number }

export function Team() {
  const me = getUser()
  const [users, setUsers] = useState<User[]>([])
  const [settings, setSettings] = useState<Settings | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const isAdmin = me?.role === 'admin'

  async function load() {
    try {
      const [us, s] = await Promise.all([
        api<Page<User>>('GET', '/users?size=50'),
        api<Settings>('GET', '/settings'),
      ])
      setUsers(us.items)
      setSettings(s)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function createUser(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/users', {
        email: fd.get('email'),
        password: fd.get('password'),
        full_name: fd.get('full_name'),
        role: fd.get('role'),
      })
      ;(e.currentTarget as HTMLFormElement).reset()
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create user failed')
    } finally {
      setBusy(false)
    }
  }

  async function toggle(u: User) {
    try {
      await api('PATCH', `/users/${u.id}`, { is_active: !u.is_active })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Update failed')
    }
  }

  async function saveSettings(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const fd = new FormData(e.currentTarget)
    try {
      const s = await api<Settings>('PUT', '/settings', {
        portal_title: fd.get('portal_title') || '',
        portal_message: fd.get('portal_message') || '',
      })
      setSettings(s)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Save failed')
    }
  }

  async function changePassword(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/change-password', { old_password: fd.get('old'), new_password: fd.get('new') })
      ;(e.currentTarget as HTMLFormElement).reset()
      setError('Password updated')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Change failed')
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h2>Team & Settings</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <div className="grid-2">
        <div className="card">
          <h3>Members</h3>
          {!isAdmin && (
            <form className="grid-form" onSubmit={createUser}>
              <label>
                Full name
                <input name="full_name" required />
              </label>
              <label>
                Email
                <input name="email" type="email" required />
              </label>
              <label>
                Password
                <input name="password" type="password" minLength={8} required />
              </label>
              <label>
                Role
                <select name="role">
                  <option value="staff">Staff</option>
                  <option value="owner">Owner</option>
                </select>
              </label>
              <div className="form-actions">
                <button className="btn primary" type="submit" disabled={busy}>
                  Add member
                </button>
              </div>
            </form>
          )}

          <div className="table-wrap">
            <table className="table">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th>Created</th>
                  {!isAdmin && <th className="right">Actions</th>}
                </tr>
              </thead>
              <tbody>
                {users.map((u) => (
                  <tr key={u.id}>
                    <td>{u.full_name}</td>
                    <td className="muted">{u.email}</td>
                    <td>{u.role}</td>
                    <td>
                      <span className={`badge ${u.is_active ? 'ok' : 'warn'}`}>{u.is_active ? 'active' : 'inactive'}</span>
                    </td>
                    <td className="muted">{relTime(u.created_at)}</td>
                    {!isAdmin && (
                      <td className="right">
                        {u.id !== me?.id && (
                          <button className="btn mini" onClick={() => void toggle(u)}>
                            {u.is_active ? 'Disable' : 'Enable'}
                          </button>
                        )}
                      </td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="stack">
          <form className="card" onSubmit={saveSettings}>
            <h3>Captive portal</h3>
            <label>
              Portal title
              <input name="portal_title" defaultValue={settings?.portal_title ?? ''} placeholder="Selamat datang di MyWiFi" />
            </label>
            <label>
              Portal message
              <textarea name="portal_message" defaultValue={settings?.portal_message ?? ''} rows={3} />
            </label>
            <button className="btn primary" type="submit">
              Save settings
            </button>
          </form>

          <form className="card" onSubmit={changePassword}>
            <h3>Change my password</h3>
            <label>
              Current password
              <input name="old" type="password" required />
            </label>
            <label>
              New password
              <input name="new" type="password" minLength={8} required />
            </label>
            <button className="btn primary" type="submit">
              Update password
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}

export default Team