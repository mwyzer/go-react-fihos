import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { fmtDur, fmtTime } from '../lib/fmt'

type Batch = {
  id: number
  name: string
  quantity: number
  price: number | null
  duration: number
  profile_id: number
  valid_from: string | null
  valid_to: string | null
  created_at: string
  unused: number
  redeemed: number
  expired: number
  revoked: number
}

type Voucher = {
  id: number
  code: string
  status: string
  redeemed_at: string | null
  expires_at: string | null
}

type Page<T> = { items: T[]; total: number; page: number; size: number }

export function Vouchers() {
  const [batches, setBatches] = useState<Batch[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [error, setError] = useState('')
  const [open, setOpen] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  async function load() {
    try {
      const res = await api<Page<Batch>>('GET', `/batches?page=${page}&size=10`)
      setBatches(res.items ?? [])
      setTotal(res.total)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load batches')
    }
  }

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void load()
  }, [page])

  async function create(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    const fd = new FormData(e.currentTarget)
    try {
      const price = Number(fd.get('price') || 0)
      await api('POST', '/batches', {
        name: fd.get('name') || undefined,
        quantity: Number(fd.get('quantity')),
        price,
        duration: Number(fd.get('duration')),
        profile_id: Number(fd.get('profile_id')),
        valid_from: fd.get('valid_from') ? new Date(String(fd.get('valid_from'))).toISOString() : undefined,
        valid_to: fd.get('valid_to') ? new Date(String(fd.get('valid_to'))).toISOString() : undefined,
      })
      ;(e.currentTarget as HTMLFormElement).reset()
      setPage(1)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create batch')
    } finally {
      setBusy(false)
    }
  }

  async function revoke(vid: number) {
    if (!open) return
    try {
      await api('POST', `/batches/${open}/vouchers/${vid}/revoke`)
      setOpen(null)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Revoke failed')
    }
  }

  const largest = total > 0 ? Math.ceil(total / 10) : 1
  const [creating, setCreating] = useState(false)

  return (
    <div className="page">
      <header className="page-head">
        <h2>Voucher batches</h2>
        <button className="btn ghost" onClick={() => void load()}>
          Refresh
        </button>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <button className="btn primary" onClick={() => setCreating((v) => !v)}>
        {creating ? 'Cancel' : '+ New batch'}
      </button>

      {creating && (
        <form className="card grid-form" onSubmit={create}>
          <label>
            Batch name
            <input name="name" placeholder="Automatic if empty" />
          </label>
          <label>
            Quantity
            <input name="quantity" type="number" min={1} max={500} required />
          </label>
          <label>
            Price (Rp)
            <input name="price" type="number" min={0} step="100" defaultValue={0} />
          </label>
          <label>
            Duration (minutes)
            <input name="duration" type="number" min={1} required />
          </label>
          <label>
            Profile ID
            <input name="profile_id" type="number" min={1} required />
          </label>
          <label>
            Valid from
            <input name="valid_from" type="datetime-local" />
          </label>
          <label>
            Valid to
            <input name="valid_to" type="datetime-local" />
          </label>
          <div className="form-actions">
            <button className="btn primary" type="submit" disabled={busy}>
              {busy ? 'Creating…' : 'Create batch'}
            </button>
          </div>
        </form>
      )}

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Batch</th>
              <th>Duration</th>
              <th>Price</th>
              <th>Unused</th>
              <th>Redeemed</th>
              <th>Expired</th>
              <th>Revoked</th>
              <th>Created</th>
            </tr>
          </thead>
          <tbody>
            {batches.map((b) => (
              <tr key={b.id} className={open === b.id ? 'row-active' : ''}>
                <td>
                  <button className="link" onClick={() => setOpen(open === b.id ? null : b.id)}>
                    {b.name}
                  </button>
                </td>
                <td>{fmtDur(b.duration)}</td>
                <td>{b.price ? `Rp ${b.price.toLocaleString('id-ID')}` : 'Free'}</td>
                <td>{b.unused}</td>
                <td>{b.redeemed}</td>
                <td>{b.expired}</td>
                <td>{b.revoked}</td>
                <td className="muted">{fmtTime(b.created_at)}</td>
              </tr>
            ))}
            {batches.length === 0 && (
              <tr>
                <td colSpan={8} className="muted">
                  No batches yet.
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

      {open && <BatchDetail batchId={open} onRevoke={revoke} />}
    </div>
  )
}

function BatchDetail({ batchId, onRevoke }: { batchId: number; onRevoke: (vid: number) => Promise<void> }) {
  const [vouchers, setVouchers] = useState<Voucher[]>([])
  const [error, setError] = useState('')
  const [q, setQ] = useState('')

  useEffect(() => {
    const u = getUser()
    if (u != null && u.tenant_id == null && getActiveTenant() == null) return
    void (async () => {
      try {
        const res = await api<Page<Voucher>>('GET', `/batches/${batchId}/vouchers?size=200${q ? `&q=${encodeURIComponent(q)}` : ''}`)
        setVouchers(res.items ?? [])
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to list vouchers')
      }
    })()
  }, [batchId, q])

  return (
    <div className="card">
      <div className="row-space">
        <h3>Vouchers</h3>
        <input placeholder="Search code…" value={q} onChange={(e) => setQ(e.target.value)} style={{ maxWidth: 240 }} />
      </div>
      {error && <div className="error-banner">{error}</div>}
      <div className="grp">
        {vouchers.map((v) => (
          <div className="voucher" key={v.id}>
            <span className="code">{v.code}</span>
            <span className={`badge ${v.status}`}>{v.status}</span>
            {v.status === 'unused' && (
              <button className="btn mini" onClick={() => void onRevoke(v.id)}>
                Revoke
              </button>
            )}
          </div>
        ))}
        {vouchers.length === 0 && <p className="muted">No matching vouchers.</p>}
      </div>
    </div>
  )
}

export default Vouchers