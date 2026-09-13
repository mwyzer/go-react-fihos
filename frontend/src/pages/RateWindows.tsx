import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import { fmtTime } from '../lib/fmt'

type RateWindow = {
  id: number
  hotspot_id: number
  hotspot_name: string
  boost_multiplier: number
  effective_from: string
  effective_until: string
  note: string | null
  created_at: string
}

type Hotspot = { id: number; name: string }

export function RateWindows() {
  const [items, setItems] = useState<RateWindow[]>([])
  const [hotspots, setHotspots] = useState<Hotspot[]>([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function load() {
    try {
      const [w, hs] = await Promise.all([api<RateWindow[]>('GET', '/rate-windows'), api<Hotspot[]>('GET', '/hotspots')])
      setItems(w)
      setHotspots(hs)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    }
  }

  useEffect(() => {
    void load()
  }, [])

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/rate-windows', {
        hotspot_id: Number(fd.get('hotspot_id')),
        boost_multiplier: Number(fd.get('boost_multiplier')),
        effective_from: new Date(String(fd.get('effective_from'))).toISOString(),
        effective_until: new Date(String(fd.get('effective_until'))).toISOString(),
      })
      ;(e.currentTarget as HTMLFormElement).reset()
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create failed')
    } finally {
      setBusy(false)
    }
  }

  async function del(id: number) {
    try {
      await api('DELETE', `/rate-windows/${id}`)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
    }
  }

  const now = new Date().toISOString().slice(0, 16)

  return (
    <div className="page">
      <header className="page-head">
        <h2>Rate boost windows</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <form className="card grid-form" onSubmit={create}>
        <label>
          Hotspot
          <select name="hotspot_id" required>
            <option value="" disabled>
              Select hotspot…
            </option>
            {hotspots.map((h) => (
              <option key={h.id} value={h.id}>
                {h.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Boost multiplier
          <input name="boost_multiplier" type="number" min={1.1} step="0.1" defaultValue={2} required />
        </label>
        <label>
          From
          <input name="effective_from" type="datetime-local" min={now} required />
        </label>
        <label>
          Until
          <input name="effective_until" type="datetime-local" min={now} required />
        </label>
        <div className="form-actions">
          <button className="btn primary" type="submit" disabled={busy}>
            {busy ? 'Creating…' : 'Create window'}
          </button>
        </div>
      </form>

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Hotspot</th>
              <th>Multiplier</th>
              <th>From</th>
              <th>Until</th>
              <th>Status</th>
              <th className="right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.map((w) => {
              const live = new Date(w.effective_from).getTime() <= Date.now() && new Date(w.effective_until).getTime() > Date.now()
              const future = new Date(w.effective_from).getTime() > Date.now()
              return (
                <tr key={w.id}>
                  <td>{w.hotspot_name}</td>
                  <td>
                    <span className={`badge ${live ? 'boost' : future ? 'info' : 'dim'}`}>x{w.boost_multiplier}</span>
                  </td>
                  <td className="muted">{fmtTime(w.effective_from)}</td>
                  <td className="muted">{fmtTime(w.effective_until)}</td>
                  <td>
                    <span className={`badge ${live ? 'ok' : future ? 'info' : 'dim'}`}>
                      {live ? 'live' : future ? 'scheduled' : 'ended'}
                    </span>
                  </td>
                  <td className="right">
                    {!live && (
                      <button className="btn mini" onClick={() => void del(w.id)}>
                        Delete
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
            {items.length === 0 && (
              <tr>
                <td colSpan={6} className="muted">
                  No rate windows.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default RateWindows