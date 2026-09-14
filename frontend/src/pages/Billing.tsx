import { useEffect, useState } from 'react'
import { api, getActiveTenant, getUser } from '../lib/api'
import { fmtDate } from '../lib/fmt'

type Window = {
  id: number
  customer_id: number
  customer_name: string
  period_start: string
  period_end: string
  amount: number
  status: 'draft' | 'issued' | 'paid' | 'overdue'
  issued_at: string
  paid_at: string | null
}

type Page<T> = { items: T[]; total: number; page: number; size: number }
type Overview = { monthly_fee: number; pending: Record<string, number>; amount_due: number }

const STATUS_LABEL: Record<string, string> = { draft: 'Draft', issued: 'Tagihan', paid: 'Lunas', overdue: 'Overdue' }

export function Billing() {
  const me = getUser()
  const isOwner = me?.role === 'admin' || me?.role === 'owner'
  const [items, setItems] = useState<Window[]>([])
  const [filter, setFilter] = useState('')
  const [ov, setOv] = useState<Overview | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function load() {
    try {
      const params = new URLSearchParams({ size: '100' })
      if (filter) params.set('status', filter)
      const [page, o] = await Promise.all([
        api<Page<Window>>('GET', `/billing?${params}`),
        api<Overview>('GET', '/billing/overview'),
      ])
      setItems(page.items ?? [])
      setOv(o)
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

  async function generate() {
    setBusy(true)
    try {
      await api('POST', '/billing/generate')
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Generate failed')
    } finally {
      setBusy(false)
    }
  }

  async function pay(w: Window, method: 'wallet' | 'manual') {
    try {
      await api('POST', `/billing/${w.id}/pay`, { method })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Mark paid failed')
    }
  }

  return (
    <div className="page">
      <header className="page-head">
        <h2>Billing windows</h2>
        <div className="head-actions">
          {ov && (
            <span className="muted small">
              Fee {new Intl.NumberFormat('id-ID').format(ov.monthly_fee)} · Piutang{' '}
              {new Intl.NumberFormat('id-ID').format(ov.amount_due)}
            </span>
          )}
          {isOwner && (
            <button className="btn primary" onClick={() => void generate()} disabled={busy}>
              {busy ? 'Generating…' : 'Generate invoice bulanan'}
            </button>
          )}
        </div>
      </header>

      {error && <div className="error-banner">{error}</div>}

      <div className="filters">
        {(['issued', 'paid', 'overdue', 'draft'] as const).map((s) => (
          <button key={s} className={`btn ghost small ${filter === s ? 'active' : ''}`} onClick={() => setFilter(filter === s ? '' : s)}>
            {STATUS_LABEL[s]} {ov ? `(${ov.pending[s] ?? 0})` : ''}
          </button>
        ))}
        {filter && (
          <button className="btn ghost small" onClick={() => setFilter('')}>
            x
          </button>
        )}
      </div>

      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th>Pelanggan</th>
              <th>Periode</th>
              <th>Jumlah</th>
              <th>Status</th>
              {isOwner && <th className="right">Aksi</th>}
            </tr>
          </thead>
          <tbody>
            {items.map((w) => (
              <tr key={w.id}>
                <td>{w.customer_name}</td>
                <td className="muted">
                  {fmtDate(w.period_start)} → {fmtDate(w.period_end)}
                </td>
                <td>{new Intl.NumberFormat('id-ID').format(w.amount)}</td>
                <td>
                  <span className={`badge ${w.status === 'paid' ? 'ok' : w.status === 'overdue' ? 'warn' : w.status === 'issued' ? 'info' : 'dim'}`}>
                    {STATUS_LABEL[w.status]}
                  </span>
                </td>
                {isOwner && (
                  <td className="right">
                    {w.status !== 'paid' && (
                      <>
                        <button className="btn mini" onClick={() => void pay(w, 'wallet')}>
                          Bayar saldo
                        </button>
                        <button className="btn mini" onClick={() => void pay(w, 'manual')}>
                          Manual
                        </button>
                      </>
                    )}
                  </td>
                )}
              </tr>
            ))}
            {items.length === 0 && (
              <tr>
                <td colSpan={isOwner ? 5 : 4} className="muted">
                  Belum ada tagihan. Klik "Generate invoice bulanan" untuk pelanggan berstatus token.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default Billing