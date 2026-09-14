import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { fmtBytes, fmtDur, relTime } from '../lib/fmt'

type Session = {
  id: number
  hotspot_id: number
  state: string
  username: string
  mac_address: string | null
  ip_address: string | null
  started_at: string
  last_seen_at: string | null
  end_time: string | null
  bytes_rx: number
  bytes_tx: number
}

type Page<T> = { items: T[]; total: number; page: number; size: number }

export function Sessions() {
  const [items, setItems] = useState<Session[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [state, setState] = useState('')
  const [q, setQ] = useState('')
  const [error, setError] = useState('')

  async function load() {
    try {
      const qs = new URLSearchParams({ page: String(page), size: '15' })
      if (state) qs.set('state', state)
      if (q) qs.set('q', q)
      const res = await api<Page<Session>>('GET', `/sessions?${qs.toString()}`)
      setItems(res.items ?? [])
      setTotal(res.total)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sessions')
    }
  }

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [page, state])

  async function disconnect(id: number) {
    try {
      await api('POST', `/sessions/${id}/disconnect`)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Disconnect failed')
    }
  }

  const largest = Math.max(1, Math.ceil(total / 15))

  return (
    <div className="page">
      <header className="page-head">
        <h2>Sessions</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <div className="row-space">
        <div className="row-gap">
          <select value={state} onChange={(e) => { setState(e.target.value); setPage(1) }}>
            <option value="">All states</option>
            <option value="active">Active</option>
            <option value="closing">Closing</option>
            <option value="closed">Closed</option>
          </select>
          <input
            placeholder="Search username…"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') { setPage(1); void load() } }}
          />
        </div>
        <span className="muted">{total} total</span>
      </div>

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Username</th>
              <th>State</th>
              <th>MAC</th>
              <th>IP</th>
              <th>Started</th>
              <th>Last seen</th>
              <th>Usage</th>
              <th className="right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.map((s) => (
              <tr key={s.id}>
                <td>{s.username}</td>
                <td>
                  <span className={`badge ${s.state === 'active' ? 'ok' : 'dim'}`}>{s.state}</span>
                </td>
                <td className="muted">{s.mac_address ?? '—'}</td>
                <td className="muted">{s.ip_address ?? '—'}</td>
                <td className="muted">{relTime(s.started_at)}</td>
                <td className="muted">{relTime(s.last_seen_at)}</td>
                <td>
                  {fmtBytes(s.bytes_rx + s.bytes_tx)}
                  <span className="muted small"> / {fmtDur(s.end_time ? undefined : undefined)}</span>
                </td>
                <td className="right">
                  {s.state !== 'closed' && (
                    <button className="btn mini" onClick={() => void disconnect(s.id)}>
                      Disconnect
                    </button>
                  )}
                </td>
              </tr>
            ))}
            {items.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  No sessions found.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {largest > 1 && (
        <div className="pager">
          <button className="btn mini" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
            Prev
          </button>
          <span className="muted">
            Page {page} / {largest}
          </span>
          <button className="btn mini" disabled={page >= largest} onClick={() => setPage((p) => p + 1)}>
            Next
          </button>
        </div>
      )}
    </div>
  )
}

export default Sessions