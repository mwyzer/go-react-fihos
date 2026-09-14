import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { fmtDur, relTime } from '../lib/fmt'

type Alert = {
  id: number
  hotspot_id: number | null
  hotspot_name: string
  type: string
  severity: string
  status: string
  value: number
  baseline: number
  occurrences: number
  created_at: string
  resolved_at: string | null
}

type Page<T> = { items: T[]; total: number; page: number; size: number }

const typeLabel: Record<string, string> = {
  excess_traffic: 'Excess traffic',
  voucher_shared: 'Voucher shared',
  traffic_spike: 'Traffic spike',
  concurrency_gap: 'Concurrency gap',
  over_limit_session: 'Over-limit session',
  mac_hop: 'MAC hop',
}

export function Alerts() {
  const [items, setItems] = useState<Alert[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('')
  const [error, setError] = useState('')

  async function load() {
    try {
      const qs = new URLSearchParams({ page: String(page), size: '15' })
      if (statusFilter) qs.set('status', statusFilter)
      const res = await api<Page<Alert>>('GET', `/alerts?${qs.toString()}`)
      setItems(res.items ?? [])
      setTotal(res.total)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load alerts')
    }
  }

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [page, statusFilter])

  async function updateStatus(id: number, s: string) {
    try {
      await api('PATCH', `/alerts/${id}/status`, { status: s })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Update failed')
    }
  }

  const largest = Math.max(1, Math.ceil(total / 15))

  return (
    <div className="page">
      <header className="page-head">
        <h2>Anomaly alerts</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <div className="row-space">
        <select value={statusFilter} onChange={(e) => { setStatusFilter(e.target.value); setPage(1) }}>
          <option value="">All statuses</option>
          <option value="open">Open</option>
          <option value="acknowledged">Acknowledged</option>
          <option value="resolved">Resolved</option>
        </select>
        <span className="muted">{total} total</span>
      </div>

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Type</th>
              <th>Severity</th>
              <th>Hotspot</th>
              <th>Value / baseline</th>
              <th>Occurrences</th>
              <th>Created</th>
              <th>Status</th>
              <th className="right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items.map((a) => (
              <tr key={a.id}>
                <td>{typeLabel[a.type] ?? a.type}</td>
                <td>
                  <span className={`badge sev-${a.severity}`}>{a.severity}</span>
                </td>
                <td>{a.hotspot_name || '—'}</td>
                <td className="muted">
                  {a.value > 0 && a.baseline > 0 ? `${fmtDur(Math.round(a.value))} / ${fmtDur(Math.round(a.baseline))}` : `${a.value.toFixed(1)} / ${a.baseline.toFixed(1)}`}
                </td>
                <td>{a.occurrences}</td>
                <td className="muted">{relTime(a.created_at)}</td>
                <td>
                  <span className={`badge ${a.status === 'open' ? 'warn' : a.status === 'acknowledged' ? 'info' : 'ok'}`}>{a.status}</span>
                </td>
                <td className="right">
                  {a.status === 'open' && (
                    <>
                      <button className="btn mini" onClick={() => void updateStatus(a.id, 'acknowledged')}>
                        Ack
                      </button>
                      <button className="btn mini" onClick={() => void updateStatus(a.id, 'resolved')}>
                        Resolve
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))}
            {items.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  No alerts.
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

export default Alerts