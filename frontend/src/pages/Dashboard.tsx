import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { fmtBytes, fmtDate } from '../lib/fmt'

type DashboardData = {
  sessions_active: number
  sessions_today: number
  vouchers_sold: number
  revenue_today: number
  traffic_today: number
  total_hotspots: number
  active_hotspots: number
  online_routers: number
  open_alerts: number
}

type DailyRow = {
  day: string
  sessions: number
  vouchers_sold: number
  bytes_rx: number
  bytes_tx: number
  amount: number
}

type TopHotspot = { hotspot_id: number; name: string; bytes_rx: number; bytes_tx: number; sessions: number }

export function Dashboard() {
  const [dash, setDash] = useState<DashboardData | null>(null)
  const [series, setSeries] = useState<DailyRow[]>([])
  const [top, setTop] = useState<TopHotspot[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [])

  async function load() {
    try {
      const res = await api<{ dashboard: DashboardData; series: DailyRow[] | null; top: TopHotspot[] | null }>('GET', '/dashboard')
      setDash(res.dashboard)
      setSeries(res.series ?? [])
      setTop(res.top ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load dashboard')
    }
  }

  if (error) return <div className="error-banner">{error}</div>
  if (!dash) return <div className="muted">Loading…</div>

  const cards: { label: string; value: string; sub?: string }[] = [
    { label: 'Active sessions', value: String(dash.sessions_active) },
    { label: 'Sessions today', value: String(dash.sessions_today) },
    { label: 'Vouchers sold', value: String(dash.vouchers_sold) },
    { label: 'Revenue today', value: `Rp ${dash.revenue_today.toLocaleString('id-ID', { maximumFractionDigits: 0 })}` },
    { label: 'Traffic today', value: fmtBytes(dash.traffic_today) },
    { label: 'Hotspots active', value: `${dash.active_hotspots} / ${dash.total_hotspots}` },
    { label: 'Routers online', value: String(dash.online_routers) },
    { label: 'Open alerts', value: String(dash.open_alerts) },
  ]

  const maxBytes = Math.max(...series.map((r) => r.bytes_rx + r.bytes_tx), 1)

  return (
    <div className="page">
      <header className="page-head">
        <h2>Dashboard</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      <div className="stats">
        {cards.map((card) => (
          <div className="card stat" key={card.label}>
            <div className="stat-label">{card.label}</div>
            <div className="stat-value">{card.value}</div>
            {card.sub && <div className="muted small">{card.sub}</div>}
          </div>
        ))}
      </div>

      <div className="grid-2">
        <div className="card">
          <h3>Traffic last 14 days</h3>
          <div className="bars">
            {series.map((r) => (
              <div className="bar-col" key={r.day} title={`${fmtDate(r.day)} — ${fmtBytes(r.bytes_rx + r.bytes_tx)}`}>
                <div
                  className="bar"
                  style={{ height: `${Math.max(2, ((r.bytes_rx + r.bytes_tx) / maxBytes) * 100)}%` }}
                />
              </div>
            ))}
          </div>
        </div>

        <div className="card">
          <h3>Top hotspots</h3>
          <table className="table">
            <thead>
              <tr>
                <th>Hotspot</th>
                <th>Sessions</th>
                <th>Traffic</th>
              </tr>
            </thead>
            <tbody>
              {top.map((t) => (
                <tr key={t.hotspot_id}>
                  <td>{t.name}</td>
                  <td>{t.sessions}</td>
                  <td>{fmtBytes(t.bytes_rx + t.bytes_tx)}</td>
                </tr>
              ))}
              {top.length === 0 && (
                <tr>
                  <td colSpan={3} className="muted">
                    No traffic yet
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

export default Dashboard