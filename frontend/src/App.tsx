import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser, logout, setActiveTenant } from './lib/api'
import { useHashRoute, navigate } from './lib/router'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import Infra from './pages/Infra'
import Vouchers from './pages/Vouchers'
import Sessions from './pages/Sessions'
import Alerts from './pages/Alerts'
import RateWindows from './pages/RateWindows'
import Team from './pages/Team'

const nav = [
  { path: '/dashboard', label: 'Dashboard' },
  { path: '/routers', label: 'Routers & Hotspots' },
  { path: '/vouchers', label: 'Vouchers' },
  { path: '/sessions', label: 'Sessions' },
  { path: '/alerts', label: 'Anomalies' },
  { path: '/rate-windows', label: 'Rate Boost' },
  { path: '/team', label: 'Team & Settings' },
]

type TenantOpt = { id: number; name: string; slug: string; status: string }

function TenantPicker({ onPick }: { onPick: () => void }) {
  const [tenants, setTenants] = useState<TenantOpt[]>([])
  const [value, setValue] = useState<number | null>(() => getActiveTenant())

  useEffect(() => {
    void api<{ items: TenantOpt[] }>('GET', '/admin/tenants')
      .then((res) => {
        setTenants(res.items)
        const active = getActiveTenant() ?? res.items[0]?.id ?? null
        if (active !== getActiveTenant()) {
          setActiveTenant(active)
          setValue(active)
          onPick()
        } else {
          setValue(active)
        }
      })
      .catch(() => setTenants([]))
    // run once on mount; onPick is stable enough via setTick
  }, [])

  function onSelect(id: string) {
    const v = Number(id) || null
    setActiveTenant(v)
    setValue(v)
    onPick()
  }

  return (
    <label className="tenant-picker">
      <span className="muted small">Tenant</span>
      <select value={value ?? ''} onChange={(e) => onSelect(e.target.value)}>
        <option value="">— pilih tenant —</option>
        {tenants.map((t) => (
          <option key={t.id} value={t.id}>
            {t.name}
          </option>
        ))}
      </select>
    </label>
  )
}

export function App() {
  const route = useHashRoute()
  const [me, setMe] = useState(() => getUser())
  const [tick, setTick] = useState(0)

  if (!me) {
    return <Login onLogin={() => setMe(getUser())} />
  }

  const page = route.split('?')[0]
  const isAdmin = me.role === 'admin'
  let content: React.ReactNode
  switch (page) {
    case '/dashboard':
      content = <Dashboard />
      break
    case '/routers':
      content = <Infra />
      break
    case '/vouchers':
      content = <Vouchers />
      break
    case '/sessions':
      content = <Sessions />
      break
    case '/alerts':
      content = <Alerts />
      break
    case '/rate-windows':
      content = <RateWindows />
      break
    case '/team':
      content = <Team />
      break
    default:
      content = <Dashboard />
      break
  }

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand-row">
          <span className="brand">FIHOS</span>
        </div>
        <nav className="nav">
          {nav.map((n) => (
            <button
              key={n.path}
              className={`nav-item ${page === n.path ? 'active' : ''}`}
              onClick={() => navigate(n.path)}
            >
              {n.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-foot">
          <div className="muted small truncate">
            {me.full_name} · {me.role}
          </div>
          {isAdmin && <TenantPicker onPick={() => setTick((n) => n + 1)} />}
          <button className="btn ghost small" onClick={logout}>
            Sign out
          </button>
        </div>
      </aside>
      <main className="content" key={tick}>{content}</main>
    </div>
  )
}

export default App