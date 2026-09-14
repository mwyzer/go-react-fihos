import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { relTime } from '../lib/fmt'

type Router = {
  id: number
  name: string
  ip_address: string
  api_port: number
  username: string
  status: string
  last_seen_at: string | null
  last_sync_at: string | null
}

type Hotspot = {
  id: number
  name: string
  router_id: number
  profile_id: number
  mikrotik_id: string | null
  ip_range: string | null
  status: string
  applied_multiplier: number
}

type Profile = { id: number; name: string; rx_rate: number; tx_rate: number; session_uptime_limit: number }

export function Infra() {
  const [routers, setRouters] = useState<Router[]>([])
  const [hotspots, setHotspots] = useState<Hotspot[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [error, setError] = useState('')
  const [active, setActive] = useState<{ r: Router; hs: Hotspot[] } | null>(null)

  async function load() {
    try {
      const [rs, hs, ps] = await Promise.all([
        api<Router[]>('GET', '/routers'),
        api<Hotspot[]>('GET', '/hotspots'),
        api<Profile[]>('GET', '/profiles'),
      ])
      setRouters(rs ?? [])
      setHotspots(hs ?? [])
      setProfiles(ps ?? [])
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    }
  }

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [])

  async function createRouter(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/routers', {
        name: fd.get('name'),
        ip_address: fd.get('ip') || '192.168.88.1',
        api_port: Number(fd.get('port') || 8728),
        username: fd.get('username') || 'admin',

        password: fd.get('password') || '',
      })
      ;(e.currentTarget as HTMLFormElement).reset()
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create router')
    }
  }

  async function probe(id: number) {
    try {
      await api('POST', `/routers/${id}/probe`)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Probe failed')
      await load()
    }
  }

  async function simulate(id: number, connected: boolean) {
    try {
      await api('POST', `/routers/${id}/simulate`, { connected })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Toggle failed')
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h2>Routers & Hotspots</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <form className="card inline-form" onSubmit={createRouter}>
        <input name="name" placeholder="Router name" required />
        <input name="ip" placeholder="IP address" defaultValue="192.168.88.1" />
        <input name="port" placeholder="API port" defaultValue="8728" style={{ maxWidth: 100 }} />
        <input name="username" placeholder="API username" defaultValue="admin" />

        <input name="password" type="password" placeholder="API password" autoComplete="new-password" />
        <button className="btn primary" type="submit">
          Add router
        </button>
      </form>

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Router</th>
              <th>Address</th>
              <th>Status</th>
              <th>Last seen</th>
              <th className="right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {routers.map((r) => (
              <tr key={r.id} className={active?.r.id === r.id ? 'row-active' : ''}>
                <td>
                  <button className="link" onClick={() => setActive({ r, hs: hotspots.filter((h) => h.router_id === r.id) })}>
                    {r.name}
                  </button>
                </td>
                <td className="muted">
                  {r.ip_address}:{r.api_port}
                </td>
                <td>
                  <span className={`badge ${r.status === 'online' ? 'ok' : 'warn'}`}>{r.status}</span>
                </td>
                <td className="muted">{relTime(r.last_seen_at)}</td>
                <td className="right">
                  <button className="btn mini" onClick={() => void probe(r.id)}>
                    Probe
                  </button>
                  <button className="btn mini" onClick={() => void simulate(r.id, r.status !== 'online')}>
                    {r.status === 'online' ? 'Go offline' : 'Go online'}
                  </button>
                </td>
              </tr>
            ))}
            {routers.length === 0 && (
              <tr>
                <td colSpan={5} className="muted">
                  No routers yet — add one above.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {active && <HotspotPanel key={active.r.id} router={active.r} hotspots={active.hs} profiles={profiles} onChanged={load} />}
    </div>
  )
}

function HotspotPanel({
  router,
  hotspots,
  profiles,
  onChanged,
}: {
  router: Router
  hotspots: Hotspot[]
  profiles: Profile[]
  onChanged: () => Promise<void>
}) {
  const [error, setError] = useState('')
  const [show, setShow] = useState(false)

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    const fd = new FormData(e.currentTarget)
    try {
      await api('POST', '/hotspots', {
        router_id: router.id,
        profile_id: Number(fd.get('profile_id')),
        name: fd.get('name'),
        ip_range: (fd.get('ip_range') as string) || '',
      })
      setShow(false)
      await onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create hotspot')
    }
  }

  async function toggle(h: Hotspot) {
    try {
      await api('PATCH', `/hotspots/${h.id}/status`, { status: h.status === 'active' ? 'disabled' : 'active' })
      await onChanged()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Toggle failed')
    }
  }

  return (
    <div className="card">
      <div className="row-space">
        <h3>Hotspots on {router.name}</h3>
        <button className="btn mini" onClick={() => setShow((v) => !v)}>
          {show ? 'Cancel' : '+ Add hotspot'}
        </button>
      </div>
      {error && <div className="error-banner">{error}</div>}
      {show && (
        <form className="inline-form" onSubmit={create}>
          <input name="name" placeholder="Hotspot name (SSID)" required />
          <select name="profile_id" defaultValue="" required>
            <option value="" disabled>
              Profile…
            </option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <input name="ip_range" placeholder="IP pool, e.g. 10.0.0.10-10.0.0.250" />
          <button className="btn primary" type="submit">
            Create
          </button>
        </form>
      )}
      {hotspots.length === 0 && <p className="muted">No hotspots configured on this router.</p>}
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Hotspot</th>
              <th>Profile</th>
              <th>IP range</th>
              <th>MikroTik ID</th>
              <th>Multiplier</th>
              <th>Status</th>
              <th className="right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {hotspots.map((h) => (
              <tr key={h.id}>
                <td>{h.name}</td>
                <td className="muted">{profiles.find((p) => p.id === h.profile_id)?.name ?? `#${h.profile_id}`}</td>
                <td className="muted">{h.ip_range ?? '—'}</td>
                <td className="muted">{h.mikrotik_id ?? '—'}</td>
                <td>{h.applied_multiplier > 1 ? <span className="badge boost">x{h.applied_multiplier}</span> : '1x'}</td>
                <td>
                  <span className={`badge ${h.status === 'active' ? 'ok' : 'warn'}`}>{h.status}</span>
                </td>
                <td className="right">
                  <button className="btn mini" onClick={() => void toggle(h)}>
                    {h.status === 'active' ? 'Disable' : 'Enable'}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default Infra