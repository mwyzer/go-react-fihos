import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'

type Customer = {
  id: number
  hotspot_id: number | null
  name: string
  phone: string
  address: string
  status: 'active' | 'token' | 'expired' | 'suspended'
  created_at: string
}

type Page<T> = { items: T[]; total: number; page: number; size: number }

const STATUSES = ['active', 'token', 'expired', 'suspended'] as const
const STATUS_LABEL: Record<string, string> = {
  active: 'Aktif',
  token: 'Token',
  expired: 'Expired',
  suspended: 'Suspended',
}

export function Customers() {
  const me = getUser()
  const isOwner = me?.role === 'admin' || me?.role === 'owner'
  const [items, setItems] = useState<Customer[]>([])
  const [filter, setFilter] = useState('')
  const [counts, setCounts] = useState<Record<string, number>>({})
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function load() {
    try {
      const params = new URLSearchParams({ size: '100' })
      if (filter) params.set('status', filter)
      const [page, ov] = await Promise.all([
        api<Page<Customer>>('GET', `/customers?${params}`),
        api<Record<string, number>>('GET', '/customers/overview'),
      ])
      setItems(page.items ?? [])
      setCounts(ov)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    }
  }

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [filter])

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/customers', {
        name: fd.get('name'),
        phone: fd.get('phone'),
        address: fd.get('address'),
        status: fd.get('status'),
      })
      ;(e.currentTarget as HTMLFormElement).reset()
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed')
    } finally {
      setBusy(false)
    }
  }

  async function setStatus(c: Customer, status: string) {
    try {
      await api('PATCH', `/customers/${c.id}/status`, { status })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Update failed')
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h2>Pelanggan</h2>
        <div className="head-actions">
          {Object.entries(STATUS_LABEL).map(([k, label]) => (
            <button key={k} className={`btn ghost small ${filter === k ? 'active' : ''}`} onClick={() => setFilter(filter === k ? '' : k)}>
              {label}: {counts[k] ?? 0}
            </button>
          ))}
          {filter && (
            <button className="btn ghost small" onClick={() => setFilter('')}>
              x
            </button>
          )}
        </div>
      </header>

      {error && <div className="error-banner">{error}</div>}

      {isOwner && (
        <form className="card grid-form" onSubmit={create}>
          <label>
            Nama
            <input name="name" required placeholder="Nama pelanggan" />
          </label>
          <label>
            No. HP
            <input name="phone" placeholder="08xxxxxxxxxx" />
          </label>
          <label>
            Alamat
            <input name="address" placeholder="Alamat" />
          </label>
          <label>
            Status
            <select name="status" defaultValue="active">
              {STATUSES.map((s) => (
                <option key={s} value={s}>
                  {STATUS_LABEL[s]}
                </option>
              ))}
            </select>
          </label>
          <div className="form-actions">
            <button className="btn primary" type="submit" disabled={busy}>
              {busy ? 'Menyimpan…' : 'Tambah pelanggan'}
            </button>
          </div>
        </form>
      )}

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Nama</th>
              <th>No. HP</th>
              <th>Alamat</th>
              <th>Status</th>
              {isOwner && <th className="right">Ubah status</th>}
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td className="muted">{c.phone}</td>
                <td className="muted">{c.address}</td>
                <td>
                  <span className={`badge ${c.status === 'active' ? 'ok' : c.status === 'token' ? 'info' : c.status === 'suspended' ? 'warn' : 'dim'}`}>
                    {STATUS_LABEL[c.status]}
                  </span>
                </td>
                {isOwner && (
                  <td className="right">
                    <select value={c.status} onChange={(e) => void setStatus(c, e.target.value)}>
                      {STATUSES.map((s) => (
                        <option key={s} value={s}>
                          {STATUS_LABEL[s]}
                        </option>
                      ))}
                    </select>
                  </td>
                )}
              </tr>
            ))}
            {items.length === 0 && (
              <tr>
                <td colSpan={isOwner ? 5 : 4} className="muted">
                  Belum ada pelanggan.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default Customers